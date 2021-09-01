package lib

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-network-partition/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
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
	if chaosDetails.AppDetail.Label == "" {
		return errors.Errorf("please provide the appLabel")
	}
	// Get the target pod details for the chaos execution
	targetPodList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil {
		return err
	}

	podNames := []string{}
	for _, pod := range targetPodList.Items {
		podNames = append(podNames, pod.Name)
	}
	log.Infof("Target pods list for chaos, %v", podNames)

	// generate a unique string
	runID := common.GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// collect all the data for the network policy
	np := initialize()
	if err := np.getNetworkPolicyDetails(experimentsDetails); err != nil {
		return err
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
	go abortWatcher(experimentsDetails, clients, chaosDetails, resultDetails, targetPodList, runID)

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		// creating the network policy to block the traffic
		if err := createNetworkPolicy(experimentsDetails, clients, np, runID); err != nil {
			return err
		}
		// updating chaos status to injected for the target pods
		for _, pod := range targetPodList.Items {
			common.SetTargets(pod.Name, "injected", "pod", chaosDetails)
		}
	}

	// verify the presence of network policy inside cluster
	if err := checkExistanceOfPolicy(experimentsDetails, clients, experimentsDetails.Timeout, experimentsDetails.Delay, runID); err != nil {
		return err
	}

	log.Infof("[Wait]: Wait for %v chaos duration", experimentsDetails.ChaosDuration)
	common.WaitForDuration(experimentsDetails.ChaosDuration)

	// deleting the network policy after chaos duration over
	if err := deleteNetworkPolicy(experimentsDetails, clients, targetPodList, chaosDetails, experimentsDetails.Timeout, experimentsDetails.Delay, runID); err != nil {
		return err
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

	_, err := clients.KubeClient.NetworkingV1().NetworkPolicies(experimentsDetails.AppNS).Create(np)
	return err
}

// deleteNetworkPolicy deletes the network policy and wait until the network policy deleted completely
func deleteNetworkPolicy(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, targetPodList *corev1.PodList, chaosDetails *types.ChaosDetails, timeout, delay int, runID string) error {
	name := experimentsDetails.ExperimentName + "-np-" + runID
	labels := "name=" + experimentsDetails.ExperimentName + "-np-" + runID
	if err := clients.KubeClient.NetworkingV1().NetworkPolicies(experimentsDetails.AppNS).Delete(name, &v1.DeleteOptions{}); err != nil {
		return err
	}

	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			npList, err := clients.KubeClient.NetworkingV1().NetworkPolicies(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: labels})
			if err != nil || len(npList.Items) != 0 {
				return errors.Errorf("Unable to delete the network policy, err: %v", err)
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

// checkExistanceOfPolicy validate the presence of network policy inside the application namespace
func checkExistanceOfPolicy(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, timeout, delay int, runID string) error {
	labels := "name=" + experimentsDetails.ExperimentName + "-np-" + runID

	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			npList, err := clients.KubeClient.NetworkingV1().NetworkPolicies(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: labels})
			if err != nil || len(npList.Items) == 0 {
				return errors.Errorf("no network policy found, err: %v", err)
			}
			return nil
		})
}

// abortWatcher continuosly watch for the abort signals
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, targetPodList *corev1.PodList, runID string) {
	// waiting till the abort signal recieved
	<-abort

	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		if err := checkExistanceOfPolicy(experimentsDetails, clients, 2, 1, runID); err != nil {
			log.Infof("no active network policy found, err: %v", err)
			retry--
			continue
		}

		if err := deleteNetworkPolicy(experimentsDetails, clients, targetPodList, chaosDetails, 2, 1, runID); err != nil {
			log.Errorf("unable to delete network policy, err: %v", err)
		}
	}
	// updating the chaosresult after stopped
	failStep := "Chaos injection stopped!"
	types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
	result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")
	log.Info("Chaos Revert Completed")
	os.Exit(0)
}
