package lib

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-cpu-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrepareNodeCPUHog contains preparation steps before chaos injection
func PrepareNodeCPUHog(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//set up the tunables if provided in range
	setChaosTunables(experimentsDetails)

	log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
		"Node CPU Cores":           experimentsDetails.NodeCPUcores,
		"CPU Load":                 experimentsDetails.CPULoad,
		"Node Affected Percentage": experimentsDetails.NodesAffectedPerc,
		"Sequence":                 experimentsDetails.Sequence,
	})

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Select node for node-cpu-hog
	nodesAffectedPerc, _ := strconv.Atoi(experimentsDetails.NodesAffectedPerc)
	targetNodeList, err := common.GetNodeList(experimentsDetails.TargetNodes, experimentsDetails.NodeLabel, nodesAffectedPerc, clients)
	if err != nil {
		return stacktrace.Propagate(err, "could not get node list")
	}

	log.InfoWithValues("[Info]: Details of Nodes under chaos injection", logrus.Fields{
		"No. Of Nodes": len(targetNodeList),
		"Node Names":   targetNodeList,
	})

	if experimentsDetails.EngineName != "" {
		if err := common.SetHelperData(chaosDetails, experimentsDetails.SetHelperData, clients); err != nil {
			return stacktrace.Propagate(err, "could not set helper data")
		}
	}

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(experimentsDetails, targetNodeList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, targetNodeList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// injectChaosInSerialMode stress the cpu of all the target nodes serially (one by one)
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetNodeList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	nodeCPUCores := experimentsDetails.NodeCPUcores

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	for _, appNode := range targetNodeList {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNode + " node"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		// When number of cpu cores for hogging is not defined , it will take it from node capacity
		if nodeCPUCores == "0" {
			if err := setCPUCapacity(experimentsDetails, appNode, clients); err != nil {
				return stacktrace.Propagate(err, "could not get node cpu capacity")
			}
		}

		log.InfoWithValues("[Info]: Details of Node under chaos injection", logrus.Fields{
			"NodeName":     appNode,
			"NodeCPUCores": experimentsDetails.NodeCPUcores,
		})

		experimentsDetails.RunID = stringutils.GetRunID()

		// Creating the helper pod to perform node cpu hog
		if err := createHelperPod(experimentsDetails, chaosDetails, appNode, clients); err != nil {
			return stacktrace.Propagate(err, "could not create helper pod")
		}

		appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, experimentsDetails.RunID)

		//Checking the status of helper pod
		log.Info("[Status]: Checking the status of the helper pod")
		if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			return stacktrace.Propagate(err, "could not check helper status")
		}

		common.SetTargets(appNode, "targeted", "node", chaosDetails)

		// Wait till the completion of helper pod
		log.Info("[Wait]: Waiting till the completion of the helper pod")
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.ExperimentName)
		if err != nil || podStatus == "Failed" {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			return common.HelperFailedError(err, appLabel, chaosDetails.ChaosNamespace, false)
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		if err := common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			return stacktrace.Propagate(err, "could not delete helper pod(s)")
		}
	}
	return nil
}

// injectChaosInParallelMode stress the cpu of  all the target nodes in parallel mode (all at once)
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetNodeList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	nodeCPUCores := experimentsDetails.NodeCPUcores

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	experimentsDetails.RunID = stringutils.GetRunID()

	for _, appNode := range targetNodeList {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNode + " node"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		// When number of cpu cores for hogging is not defined , it will take it from node capacity
		if nodeCPUCores == "0" {
			if err := setCPUCapacity(experimentsDetails, appNode, clients); err != nil {
				return stacktrace.Propagate(err, "could not get node cpu capacity")
			}
		}

		log.InfoWithValues("[Info]: Details of Node under chaos injection", logrus.Fields{
			"NodeName":     appNode,
			"NodeCPUcores": experimentsDetails.NodeCPUcores,
		})

		// Creating the helper pod to perform node cpu hog
		if err := createHelperPod(experimentsDetails, chaosDetails, appNode, clients); err != nil {
			return stacktrace.Propagate(err, "could not create helper pod")
		}
	}

	appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, experimentsDetails.RunID)

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pods")
	if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return stacktrace.Propagate(err, "could not check helper status")
	}

	for _, appNode := range targetNodeList {
		common.SetTargets(appNode, "targeted", "node", chaosDetails)
	}

	// Wait till the completion of helper pod
	log.Info("[Wait]: Waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, common.GetContainerNames(chaosDetails)...)
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return common.HelperFailedError(err, appLabel, chaosDetails.ChaosNamespace, false)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	if err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return stacktrace.Propagate(err, "could not delete helper pod(s)")
	}

	return nil
}

//setCPUCapacity fetch the node cpu capacity
func setCPUCapacity(experimentsDetails *experimentTypes.ExperimentDetails, appNode string, clients clients.ClientSets) error {
	node, err := clients.KubeClient.CoreV1().Nodes().Get(context.Background(), appNode, v1.GetOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{nodeName: %s}", appNode), Reason: err.Error()}
	}
	experimentsDetails.NodeCPUcores = node.Status.Capacity.Cpu().String()
	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, chaosDetails *types.ChaosDetails, appNode string, clients clients.ClientSets) error {

	terminationGracePeriodSeconds := int64(experimentsDetails.TerminationGracePeriodSeconds)

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: experimentsDetails.ExperimentName + "-helper-",
			Namespace:    experimentsDetails.ChaosNamespace,
			Labels:       common.GetHelperLabels(chaosDetails.Labels, experimentsDetails.RunID, experimentsDetails.ExperimentName),
			Annotations:  chaosDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy:                 apiv1.RestartPolicyNever,
			ImagePullSecrets:              chaosDetails.ImagePullSecrets,
			NodeName:                      appNode,
			TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
			Containers: []apiv1.Container{
				{
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"stress-ng",
					},
					Args: []string{
						"--cpu",
						experimentsDetails.NodeCPUcores,
						"--cpu-load",
						experimentsDetails.CPULoad,
						"--timeout",
						strconv.Itoa(experimentsDetails.ChaosDuration),
					},
					Resources: chaosDetails.Resources,
				},
			},
		},
	}

	if len(chaosDetails.SideCar) != 0 {
		helperPod.Spec.Containers = append(helperPod.Spec.Containers, common.BuildSidecar(chaosDetails)...)
		helperPod.Spec.Volumes = append(helperPod.Spec.Volumes, common.GetSidecarVolumes(chaosDetails)...)
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(context.Background(), helperPod, v1.CreateOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("unable to create helper pod: %s", err.Error())}
	}
	return nil
}

//setChaosTunables will set up a random value within a given range of values
//If the value is not provided in range it'll set up the initial provided value.
func setChaosTunables(experimentsDetails *experimentTypes.ExperimentDetails) {
	experimentsDetails.NodeCPUcores = common.ValidateRange(experimentsDetails.NodeCPUcores)
	experimentsDetails.CPULoad = common.ValidateRange(experimentsDetails.CPULoad)
	experimentsDetails.NodesAffectedPerc = common.ValidateRange(experimentsDetails.NodesAffectedPerc)
	experimentsDetails.Sequence = common.GetRandomSequence(experimentsDetails.Sequence)
}
