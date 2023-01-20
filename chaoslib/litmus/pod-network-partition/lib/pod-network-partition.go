package lib

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-network-partition/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	inject, abort chan os.Signal
)

//PrepareAndInjectChaos contains the prepration & injection steps
func PrepareAndInjectChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	// validate the appLabels
	if chaosDetails.AppDetail == nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "provide the appLabel"}
	}

	// Get the target pod details for the chaos execution
	targetPodList, err := common.GetPodList("", 100, clients, chaosDetails)
	if err != nil {
		return stacktrace.Propagate(err, "could not get target pods")
	}

	podNames := []string{}
	for _, pod := range targetPodList.Items {
		podNames = append(podNames, pod.Name)
	}
	log.Infof("Target pods list for chaos, %v", podNames)

	// generate a unique string
	runID := stringutils.GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// collect all the data for the network policy
	np := initialize()
	if err := np.getNetworkPolicyDetails(experimentsDetails); err != nil {
		return stacktrace.Propagate(err, "could not get network policy details")
	}

	//DISPLAY THE NETWORK POLICY DETAILS
	log.InfoWithValues("The Network policy details are as follows", logrus.Fields{
		"Target Label":      np.TargetPodLabels,
		"Policy Type":       np.PolicyType,
		"PodSelector":       np.PodSelector,
		"NamespaceSelector": np.NamespaceSelector,
		"Destination IPs":   np.ExceptIPs,
		"Ports":             np.Ports,
	})

	// watching for the abort signal and revert the chaos
	go abortWatcher(experimentsDetails, clients, chaosDetails, resultDetails, &targetPodList, runID)

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:
		// creating the network policy to block the traffic
		if err := createNetworkPolicy(experimentsDetails, clients, np, runID); err != nil {
			return stacktrace.Propagate(err, "could not create network policy")
		}
		// updating chaos status to injected for the target pods
		for _, pod := range targetPodList.Items {
			common.SetTargets(pod.Name, "injected", "pod", chaosDetails)
		}
	}

	// verify the presence of network policy inside cluster
	if err := checkExistenceOfPolicy(experimentsDetails, clients, experimentsDetails.Timeout, experimentsDetails.Delay, runID); err != nil {
		return stacktrace.Propagate(err, "could not check existence of network policy")
	}

	log.Infof("[Wait]: Wait for %v chaos duration", experimentsDetails.ChaosDuration)
	common.WaitForDuration(experimentsDetails.ChaosDuration)

	// deleting the network policy after chaos duration over
	if err := deleteNetworkPolicy(experimentsDetails, clients, &targetPodList, chaosDetails, experimentsDetails.Timeout, experimentsDetails.Delay, runID); err != nil {
		return stacktrace.Propagate(err, "could not delete network policy")
	}

	// updating chaos status to reverted for the target pods
	for _, pod := range targetPodList.Items {
		common.SetTargets(pod.Name, "reverted", "pod", chaosDetails)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	return nil
}

// createNetworkPolicy creates the network policy in the application namespace
// it blocks ingress/egress traffic for the targeted application for specific/all IPs
func createNetworkPolicy(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, networkPolicy *NetworkPolicy, runID string) error {

	np := &networkv1.NetworkPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-np-" + runID,
			Namespace: experimentsDetails.AppNS,
			Labels: map[string]string{
				"name":                      experimentsDetails.ExperimentName + "-np-" + runID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
			},
		},
		Spec: networkv1.NetworkPolicySpec{
			PodSelector: v1.LabelSelector{
				MatchLabels: networkPolicy.TargetPodLabels,
			},
			PolicyTypes: networkPolicy.PolicyType,
			Egress:      networkPolicy.Egress,
			Ingress:     networkPolicy.Ingress,
		},
	}

	_, err := clients.KubeClient.NetworkingV1().NetworkPolicies(experimentsDetails.AppNS).Create(context.Background(), np, v1.CreateOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Reason: fmt.Sprintf("failed to create network policy: %s", err.Error())}
	}
	return nil
}

// deleteNetworkPolicy deletes the network policy and wait until the network policy deleted completely
func deleteNetworkPolicy(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, targetPodList *corev1.PodList, chaosDetails *types.ChaosDetails, timeout, delay int, runID string) error {
	name := experimentsDetails.ExperimentName + "-np-" + runID
	labels := "name=" + experimentsDetails.ExperimentName + "-np-" + runID
	if err := clients.KubeClient.NetworkingV1().NetworkPolicies(experimentsDetails.AppNS).Delete(context.Background(), name, v1.DeleteOptions{}); err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{name: %s, namespace: %s}", name, experimentsDetails.AppNS), Reason: fmt.Sprintf("failed to delete network policy: %s", err.Error())}
	}

	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			npList, err := clients.KubeClient.NetworkingV1().NetworkPolicies(experimentsDetails.AppNS).List(context.Background(), v1.ListOptions{LabelSelector: labels})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{labels: %s, namespace: %s}", labels, experimentsDetails.AppNS), Reason: fmt.Sprintf("failed to list network policies: %s", err.Error())}
			} else if len(npList.Items) != 0 {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{labels: %s, namespace: %s}", labels, experimentsDetails.AppNS), Reason: "network policies are not deleted within timeout"}
			}
			return nil
		})

	if err != nil {
		return err
	}

	for _, pod := range targetPodList.Items {
		common.SetTargets(pod.Name, "reverted", "pod", chaosDetails)
	}
	return nil
}

// checkExistenceOfPolicy validate the presence of network policy inside the application namespace
func checkExistenceOfPolicy(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, timeout, delay int, runID string) error {
	labels := "name=" + experimentsDetails.ExperimentName + "-np-" + runID

	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			npList, err := clients.KubeClient.NetworkingV1().NetworkPolicies(experimentsDetails.AppNS).List(context.Background(), v1.ListOptions{LabelSelector: labels})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{labels: %s, namespace: %s}", labels, experimentsDetails.AppNS), Reason: fmt.Sprintf("failed to list network policies: %s", err.Error())}
			} else if len(npList.Items) == 0 {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{labels: %s, namespace: %s}", labels, experimentsDetails.AppNS), Reason: "no network policy found with matching labels"}
			}
			return nil
		})
}

// abortWatcher continuously watch for the abort signals
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, targetPodList *corev1.PodList, runID string) {
	// waiting till the abort signal received
	<-abort

	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		if err := checkExistenceOfPolicy(experimentsDetails, clients, 2, 1, runID); err != nil {
			if error, ok := err.(cerrors.Error); ok {
				if strings.Contains(error.Reason, "no network policy found with matching labels") {
					break
				}
			}
			log.Infof("no active network policy found, err: %v", err.Error())
			retry--
			continue
		}

		if err := deleteNetworkPolicy(experimentsDetails, clients, targetPodList, chaosDetails, 2, 1, runID); err != nil {
			log.Errorf("unable to delete network policy, err: %v", err)
		}
		retry--
	}
	// updating the chaosresult after stopped
	err := cerrors.Error{ErrorCode: cerrors.ErrorTypeExperimentAborted, Reason: "experiment is aborted"}
	failStep, errCode := cerrors.GetRootCauseAndErrorCode(err, string(chaosDetails.Phase))
	types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep, errCode)
	result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")
	log.Info("Chaos Revert Completed")
	os.Exit(0)
}
