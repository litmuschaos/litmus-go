package lib

import (
	"strconv"
	"strings"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-memory-hog/types"
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

// PrepareNodeMemoryHog contains prepration steps before chaos injection
func PrepareNodeMemoryHog(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Select node for node-memory-hog
	targetNodeList, err := common.GetNodeList(experimentsDetails.TargetNodes, experimentsDetails.NodeLabel, experimentsDetails.NodesAffectedPerc, clients)
	if err != nil {
		return err
	}
	log.InfoWithValues("[Info]: Details of Nodes under chaos injection", logrus.Fields{
		"No. Of Nodes": len(targetNodeList),
		"Node Names":   targetNodeList,
	})

	if experimentsDetails.EngineName != "" {
		if err := common.SetHelperData(chaosDetails, clients); err != nil {
			return err
		}
	}

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(experimentsDetails, targetNodeList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, targetNodeList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
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

// injectChaosInSerialMode stress the memory of all the target nodes serially (one by one)
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetNodeList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	labelSuffix := common.GetRunID()

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

		log.InfoWithValues("[Info]: Details of Node under chaos injection", logrus.Fields{
			"NodeName":                      appNode,
			"Memory Consumption Percentage": experimentsDetails.MemoryConsumptionPercentage,
			"Memory Consumption Mebibytes":  experimentsDetails.MemoryConsumptionMebibytes,
		})

		experimentsDetails.RunID = common.GetRunID()

		//Getting node memory details
		memoryCapacity, memoryAllocatable, err := getNodeMemoryDetails(appNode, clients)
		if err != nil {
			return errors.Errorf("unable to get the node memory details, err: %v", err)
		}

		//Getting the exact memory value to exhaust
		MemoryConsumption, err := calculateMemoryConsumption(experimentsDetails, clients, memoryCapacity, memoryAllocatable)
		if err != nil {
			return errors.Errorf("memory calculation failed, err: %v", err)
		}

		// Creating the helper pod to perform node memory hog
		if err = createHelperPod(experimentsDetails, chaosDetails, appNode, clients, labelSuffix, MemoryConsumption); err != nil {
			return errors.Errorf("unable to create the helper pod, err: %v", err)
		}

		appLabel := "name=" + experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID

		//Checking the status of helper pod
		log.Info("[Status]: Checking the status of the helper pod")
		if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod is not in running state, err: %v", err)
		}

		common.SetTargets(appNode, "targeted", "node", chaosDetails)

		// Wait till the completion of helper pod
		log.Info("[Wait]: Waiting till the completion of the helper pod")
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.ExperimentName)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
			return common.HelperFailedError(err)
		} else if podStatus == "Failed" {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod status is %v", podStatus)
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		if err = common.DeletePod(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			return errors.Errorf("unable to delete the helper pod, err: %v", err)
		}
	}
	return nil
}

// injectChaosInParallelMode stress the memory all the target nodes in parallel mode (all at once)
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetNodeList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	labelSuffix := common.GetRunID()

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

		log.InfoWithValues("[Info]: Details of Node under chaos injection", logrus.Fields{
			"NodeName":                      appNode,
			"Memory Consumption Percentage": experimentsDetails.MemoryConsumptionPercentage,
			"Memory Consumption Mebibytes":  experimentsDetails.MemoryConsumptionMebibytes,
		})

		experimentsDetails.RunID = common.GetRunID()

		//Getting node memory details
		memoryCapacity, memoryAllocatable, err := getNodeMemoryDetails(appNode, clients)
		if err != nil {
			return errors.Errorf("unable to get the node memory details, err: %v", err)
		}

		//Getting the exact memory value to exhaust
		MemoryConsumption, err := calculateMemoryConsumption(experimentsDetails, clients, memoryCapacity, memoryAllocatable)
		if err != nil {
			return errors.Errorf("memory calculation failed, err: %v", err)
		}

		// Creating the helper pod to perform node memory hog
		if err = createHelperPod(experimentsDetails, chaosDetails, appNode, clients, labelSuffix, MemoryConsumption); err != nil {
			return errors.Errorf("unable to create the helper pod, err: %v", err)
		}
	}

	appLabel := "app=" + experimentsDetails.ExperimentName + "-helper-" + labelSuffix

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	for _, appNode := range targetNodeList {
		common.SetTargets(appNode, "targeted", "node", chaosDetails)
	}

	// Wait till the completion of helper pod
	log.Info("[Wait]: Waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.ExperimentName)
	if err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return common.HelperFailedError(err)
	} else if podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod status is %v", podStatus)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	if err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return errors.Errorf("unable to delete the helper pod, err: %v", err)
	}

	return nil
}

// getNodeMemoryDetails will return the total memory capacity and memory allocatable of an application node
func getNodeMemoryDetails(appNodeName string, clients clients.ClientSets) (int, int, error) {

	nodeDetails, err := clients.KubeClient.CoreV1().Nodes().Get(appNodeName, v1.GetOptions{})
	if err != nil {
		return 0, 0, err
	}

	memoryCapacity := int(nodeDetails.Status.Capacity.Memory().Value())
	memoryAllocatable := int(nodeDetails.Status.Allocatable.Memory().Value())

	if memoryCapacity == 0 || memoryAllocatable == 0 {
		return memoryCapacity, memoryAllocatable, errors.Errorf("failed to get memory details of the application node")
	}

	return memoryCapacity, memoryAllocatable, nil

}

// calculateMemoryConsumption will calculate the amount of memory to be consumed for a given unit.
func calculateMemoryConsumption(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, memoryCapacity, memoryAllocatable int) (string, error) {

	var totalMemoryConsumption int
	var MemoryConsumption string
	var selector string

	if experimentsDetails.MemoryConsumptionMebibytes == 0 {
		if experimentsDetails.MemoryConsumptionPercentage == 0 {
			log.Info("Neither of MemoryConsumptionPercentage or MemoryConsumptionMebibytes provided, proceeding with a default MemoryConsumptionPercentage value of 30%%")
			return "30%", nil
		}
		selector = "percentage"
	} else {
		if experimentsDetails.MemoryConsumptionPercentage == 0 {
			selector = "mebibytes"
		} else {
			log.Warn("Both MemoryConsumptionPercentage & MemoryConsumptionMebibytes provided as inputs, using the MemoryConsumptionPercentage value to proceed with the experiment")
			selector = "percentage"
		}
	}

	switch selector {

	case "percentage":

		//Getting the total memory under chaos
		memoryForChaos := ((float64(experimentsDetails.MemoryConsumptionPercentage) / 100) * float64(memoryCapacity))

		//Get the percentage of memory under chaos wrt allocatable memory
		totalMemoryConsumption = int((float64(memoryForChaos) / float64(memoryAllocatable)) * 100)
		if totalMemoryConsumption > 100 {
			log.Infof("[Info]: PercentageOfMemoryCapacity To Be Used: %d percent, which is more than 100 percent (%d percent) of Allocatable Memory, so the experiment will only consume upto 100 percent of Allocatable Memory", experimentsDetails.MemoryConsumptionPercentage, totalMemoryConsumption)
			MemoryConsumption = "100%"
		} else {
			log.Infof("[Info]: PercentageOfMemoryCapacity To Be Used: %v percent, which is %d percent of Allocatable Memory", experimentsDetails.MemoryConsumptionPercentage, totalMemoryConsumption)
			MemoryConsumption = strconv.Itoa(totalMemoryConsumption) + "%"
		}
		return MemoryConsumption, nil

	case "mebibytes":

		// Bringing all the values in Ki unit to compare
		// since 1Mi = 1025.390625Ki
		TotalMemoryConsumption := float64(experimentsDetails.MemoryConsumptionMebibytes) * 1025.390625
		// since 1Ki = 1024 bytes
		memoryAllocatable := memoryAllocatable / 1024

		if memoryAllocatable < int(TotalMemoryConsumption) {
			MemoryConsumption = strconv.Itoa(memoryAllocatable) + "k"
			log.Infof("[Info]: The memory for consumption %vKi is more than the available memory %vKi, so the experiment will hog the memory upto %vKi", int(TotalMemoryConsumption), memoryAllocatable, memoryAllocatable)
		} else {
			MemoryConsumption = strconv.Itoa(experimentsDetails.MemoryConsumptionMebibytes) + "m"
		}
		return MemoryConsumption, nil
	}
	return "", errors.Errorf("please specify the memory consumption value either in percentage or mebibytes in a non-decimal format using respective envs")
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, chaosDetails *types.ChaosDetails, appNode string, clients clients.ClientSets, labelSuffix, MemoryConsumption string) error {

	terminationGracePeriodSeconds := int64(experimentsDetails.TerminationGracePeriodSeconds)

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID,
			Namespace:   experimentsDetails.ChaosNamespace,
			Labels:      common.GetHelperLabels(chaosDetails.Labels, experimentsDetails.RunID, labelSuffix, experimentsDetails.ExperimentName),
			Annotations: chaosDetails.Annotations,
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
						"--vm",
						strconv.Itoa(experimentsDetails.NumberOfWorkers),
						"--vm-bytes",
						MemoryConsumption,
						"--timeout",
						strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
					},
					Resources: chaosDetails.Resources,
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}
