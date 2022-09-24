package lib

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib"
	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PrepareAndInjectChaos contains the prepration and chaos injection steps
func PrepareAndInjectChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, args []string) error {

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail.Label == "" {
		return errors.Errorf("please provide one of the appLabel or TARGET_PODS")
	}
	//setup the tunables if provided in range
	litmusLIB.SetChaosTunables(experimentsDetails)

	switch experimentsDetails.NetworkChaosType {
	case "network-loss":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkPacketLossPercentage": experimentsDetails.NetworkPacketLossPercentage,
			"Sequence":                    experimentsDetails.Sequence,
			"PodsAffectedPerc":            experimentsDetails.PodsAffectedPerc,
		})
	case "network-latency":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkLatency":   strconv.Itoa(experimentsDetails.NetworkLatency),
			"Sequence":         experimentsDetails.Sequence,
			"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
		})
	case "network-corruption":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkPacketCorruptionPercentage": experimentsDetails.NetworkPacketCorruptionPercentage,
			"Sequence":                          experimentsDetails.Sequence,
			"PodsAffectedPerc":                  experimentsDetails.PodsAffectedPerc,
		})
	case "network-duplication":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkPacketDuplicationPercentage": experimentsDetails.NetworkPacketDuplicationPercentage,
			"Sequence":                           experimentsDetails.Sequence,
			"PodsAffectedPerc":                   experimentsDetails.PodsAffectedPerc,
		})
	default:
		return errors.Errorf("invalid experiment, please check the environment.go")

	}
	podsAffectedPerc, _ := strconv.Atoi(experimentsDetails.PodsAffectedPerc)
	targetPodList, err := common.GetPodList(experimentsDetails.TargetPods, podsAffectedPerc, clients, chaosDetails)
	if err != nil {
		return err
	}

	podNames := []string{}
	for _, pod := range targetPodList.Items {
		podNames = append(podNames, pod.Name)
	}
	log.Infof("Target pods list for chaos, %v", podNames)

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		if err := common.SetHelperData(chaosDetails, experimentsDetails.SetHelperData, clients); err != nil {
			return err
		}
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on target pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(experimentsDetails, targetPodList, clients, chaosDetails, args, resultDetails, eventsDetails); err != nil {
			return err
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, targetPodList, clients, chaosDetails, args, resultDetails, eventsDetails); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// injectChaosInSerialMode stress the cpu of all target application serially (one by one)
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, args []string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// creating the helper pod to perform network chaos
	for _, pod := range targetPodList.Items {

		runID := common.GetRunID()

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"Target Pod": pod.Name,
			"NodeName":   pod.Spec.NodeName,
		})
		// args contains details of the specific chaos injection
		// constructing `argsWithRegex` based on updated regex with a diff pod name
		// without extending/concatenating the args var itself
		argsWithRegex := append(args, "re2:k8s_POD_"+pod.Name+"_"+experimentsDetails.AppNS)
		log.Infof("Arguments for running %v are %v", experimentsDetails.ExperimentName, argsWithRegex)
		if err := createHelperPod(experimentsDetails, clients, chaosDetails, pod.Spec.NodeName, runID, argsWithRegex); err != nil {
			return errors.Errorf("unable to create the helper pod, err: %v", err)
		}

		appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, runID)

		//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
		log.Info("[Status]: Checking the status of the helper pod")
		if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod is not in running state, err: %v", err)
		}
		common.SetTargets(pod.Name, "targeted", "pod", chaosDetails)

		// Wait till the completion of helper pod
		log.Info("[Wait]: Waiting till the completion of the helper pod")
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, chaosDetails.ExperimentName)
		if err != nil || podStatus == "Failed" {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			return common.HelperFailedError(err)
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		if err := common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			return errors.Errorf("unable to delete the helper pod, err: %v", err)
		}
	}

	return nil
}

// injectChaosInParallelMode kill the container of all target application in parallel mode (all at once)
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, args []string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	runID := common.GetRunID()

	// creating the helper pod to perform network chaos
	for _, pod := range targetPodList.Items {

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"Target Pod": pod.Name,
			"NodeName":   pod.Spec.NodeName,
		})
		// args contains details of the specific chaos injection
		// constructing `argsWithRegex` based on updated regex with a diff pod name
		// without extending/concatenating the args var itself
		argsWithRegex := append(args, "re2:k8s_POD_"+pod.Name+"_"+experimentsDetails.AppNS)
		log.Infof("Arguments for running %v are %v", experimentsDetails.ExperimentName, argsWithRegex)
		if err := createHelperPod(experimentsDetails, clients, chaosDetails, pod.Spec.NodeName, runID, argsWithRegex); err != nil {
			return errors.Errorf("unable to create the helper pod, err: %v", err)
		}
	}

	appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, runID)

	//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}
	for _, pod := range targetPodList.Items {
		common.SetTargets(pod.Name, "targeted", "pod", chaosDetails)
	}

	// Wait till the completion of helper pod
	log.Info("[Wait]: Waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, chaosDetails.ExperimentName)
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return common.HelperFailedError(err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	if err := common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return errors.Errorf("unable to delete the helper pod, err: %v", err)
	}

	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, appNodeName, runID string, args []string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: experimentsDetails.ExperimentName + "-helper-",
			Namespace:    experimentsDetails.ChaosNamespace,
			Labels:       common.GetHelperLabels(chaosDetails.Labels, runID, experimentsDetails.ExperimentName),
			Annotations:  chaosDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy:    apiv1.RestartPolicyNever,
			ImagePullSecrets: chaosDetails.ImagePullSecrets,
			NodeName:         appNodeName,
			Volumes: []apiv1.Volume{
				{
					Name: "dockersocket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: experimentsDetails.SocketPath,
						},
					},
				},
			},
			Containers: []apiv1.Container{
				{
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"sudo",
						"-E",
					},
					Args: args,
					Env: []apiv1.EnvVar{
						{
							Name:  "DOCKER_HOST",
							Value: "unix://" + experimentsDetails.SocketPath,
						},
					},
					Resources: chaosDetails.Resources,
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "dockersocket",
							MountPath: experimentsDetails.SocketPath,
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(context.Background(), helperPod, v1.CreateOptions{})
	return err
}

// AddTargetIpsArgs inserts a comma-separated list of targetIPs (if provided by the user) into the pumba command/args
func AddTargetIpsArgs(targetIPs, targetHosts string, args []string) ([]string, error) {

	targetIPs, err := network_chaos.GetTargetIps(targetIPs, targetHosts, clients.ClientSets{}, false)
	if err != nil {
		return nil, err
	}

	if targetIPs == "" {
		return args, nil
	}
	ips := strings.Split(targetIPs, ",")
	for i := range ips {
		args = append(args, "--target", strings.TrimSpace(ips[i]))
	}
	return args, nil
}
