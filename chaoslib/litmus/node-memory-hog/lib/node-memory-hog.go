package lib

import (
	"strconv"

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
	targetNodeList, err := common.GetNodeList(experimentsDetails.TargetNodes, experimentsDetails.NodesAffectedPerc, clients)
	if err != nil {
		return err
	}
	log.InfoWithValues("[Info]: Details of Nodes under chaos injection", logrus.Fields{
		"No. Of Nodes": len(targetNodeList),
		"Node Names":   targetNodeList,
	})

	if experimentsDetails.EngineName != "" {
		// Get Chaos Pod Annotation
		experimentsDetails.Annotations, err = common.GetChaosPodAnnotation(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("unable to get annotations, err: %v", err)
		}
		// Get Resource Requirements
		experimentsDetails.Resources, err = common.GetChaosPodResourceRequirements(experimentsDetails.ChaosPodName, experimentsDetails.ExperimentName, experimentsDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("Unable to get resource requirements, err: %v", err)
		}
		// Get ImagePullSecrets
		experimentsDetails.ImagePullSecrets, err = common.GetImagePullSecrets(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("Unable to get imagePullSecrets, err: %v", err)
		}
	}

	if experimentsDetails.Sequence == "serial" {
		if err = InjectChaosInSerialMode(experimentsDetails, targetNodeList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	} else {
		if err = InjectChaosInParallelMode(experimentsDetails, targetNodeList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// InjectChaosInSerialMode stress the memory of all the target nodes serially (one by one)
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetNodeList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
		memoryCapacity, memoryAllocatable, err := GetNodeMemoryDetails(appNode, clients)
		if err != nil {
			return errors.Errorf("Unable to get the node memory details, err: %v", err)
		}

		//Getting the exact memory value to exhaust
		MemoryConsumption, err := CalculateMemoryConsumption(experimentsDetails, clients, memoryCapacity, memoryAllocatable)
		if err != nil {
			return errors.Errorf("memory calculation failed, err: %v", err)
		}

		// Creating the helper pod to perform node memory hog
		err = CreateHelperPod(experimentsDetails, appNode, clients, labelSuffix, MemoryConsumption)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}

		appLabel := "name=" + experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID

		//Checking the status of helper pod
		log.Info("[Status]: Checking the status of the helper pod")
		err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod is not in running state, err: %v", err)
		}

		// Wait till the completion of helper pod
		log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", experimentsDetails.ChaosDuration+30)

		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+30, experimentsDetails.ExperimentName)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod failed due to, err: %v", err)
		} else if podStatus == "Failed" {
			return errors.Errorf("helper pod status is %v", podStatus)
		}

		// Checking the status of target nodes
		log.Info("[Status]: Getting the status of target nodes")
		err = status.CheckNodeStatus(appNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
			log.Warnf("Target nodes are not in the ready state, you may need to manually recover the node, err: %v", err)
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		err = common.DeletePod(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
		if err != nil {
			return errors.Errorf("Unable to delete the helper pod, err: %v", err)
		}
	}
	return nil
}

// InjectChaosInParallelMode stress the memory all the target nodes in parallel mode (all at once)
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetNodeList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
		memoryCapacity, memoryAllocatable, err := GetNodeMemoryDetails(appNode, clients)
		if err != nil {
			return errors.Errorf("Unable to get the node memory details, err: %v", err)
		}

		//Getting the exact memory value to exhaust
		MemoryConsumption, err := CalculateMemoryConsumption(experimentsDetails, clients, memoryCapacity, memoryAllocatable)
		if err != nil {
			return errors.Errorf("memory calculation failed, err: %v", err)
		}

		// Creating the helper pod to perform node memory hog
		err = CreateHelperPod(experimentsDetails, appNode, clients, labelSuffix, MemoryConsumption)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}
	}

	appLabel := "app=" + experimentsDetails.ExperimentName + "-helper-" + labelSuffix

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	if err := status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", experimentsDetails.ChaosDuration+30)

	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+30, experimentsDetails.ExperimentName)
	if err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod failed due to, err: %v", err)
	} else if podStatus == "Failed" {
		return errors.Errorf("helper pod status is %v", podStatus)
	}

	for _, appNode := range targetNodeList {

		// Checking the status of application node
		log.Info("[Status]: Getting the status of application node")
		err = status.CheckNodeStatus(appNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			log.Warn("Application node is not in the ready state, you may need to manually recover the node")
		}
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	return nil
}

// GetNodeMemoryDetails will return the total memory capacity and memory allocatable of an application node
func GetNodeMemoryDetails(appNodeName string, clients clients.ClientSets) (int, int, error) {

	nodeDetails, err := clients.KubeClient.CoreV1().Nodes().Get(appNodeName, v1.GetOptions{})
	if err != nil {
		return 0, 0, err
	}

	memoryCapacity := int(nodeDetails.Status.Capacity.Memory().Value())
	memoryAllocatable := int(nodeDetails.Status.Allocatable.Memory().Value())

	if memoryCapacity == 0 || memoryAllocatable == 0 {
		return memoryCapacity, memoryAllocatable, errors.Errorf("Failed to get memory details of the application node")
	}

	return memoryCapacity, memoryAllocatable, nil

}

// CalculateMemoryConsumption will calculate the amount of memory to be consumed for a given unit.
func CalculateMemoryConsumption(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, memoryCapacity, memoryAllocatable int) (string, error) {

	var totalMemoryConsumption int
	var MemoryConsumption string
	var selector string

	if experimentsDetails.MemoryConsumptionMebibytes == 0 {
		if experimentsDetails.MemoryConsumptionPercentage == 0 {
			log.Info("Neither of MemoryConsumptionPercentage or MemoryConsumptionMebibytes provided, proceeding with a default MemoryConsumptionPercentage value of 30%")
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

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, appNode string, clients clients.ClientSets, labelSuffix, MemoryConsumption string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       experimentsDetails.ExperimentName + "-helper-" + labelSuffix,
				"name":                      experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
			},
			Annotations: experimentsDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy:    apiv1.RestartPolicyNever,
			ImagePullSecrets: experimentsDetails.ImagePullSecrets,
			NodeName:         appNode,
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
					Resources: experimentsDetails.Resources,
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}
