package lib

import (
	"strconv"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-memory-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
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
	for _, appNode := range targetNodeList {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNode + " node"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		log.InfoWithValues("[Info]: Details of Node under chaos injection", logrus.Fields{
			"NodeName":             appNode,
			"MemoryHog Percentage": experimentsDetails.MemoryPercentage,
		})

		experimentsDetails.RunID = common.GetRunID()

		//Getting node memory details
		memoryCapacity, memoryAllocatable, err := GetNodeMemoryDetails(appNode, clients)
		if err != nil {
			return errors.Errorf("Unable to get the node memory details, err: %v", err)
		}

		// Get the total memory percentage wrt allocatable memory
		experimentsDetails.MemoryPercentage = CalculateMemoryPercentage(experimentsDetails, clients, memoryCapacity, memoryAllocatable)

		// Creating the helper pod to perform node memory hog
		err = CreateHelperPod(experimentsDetails, appNode, clients, labelSuffix)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}

		appLabel := "name=" + experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID

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
		if err != nil || podStatus == "Failed" {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod failed due to, err: %v", err)
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
	for _, appNode := range targetNodeList {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNode + " node"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		log.InfoWithValues("[Info]: Details of Node under chaos injection", logrus.Fields{
			"NodeName":             appNode,
			"MemoryHog Percentage": experimentsDetails.MemoryPercentage,
		})

		experimentsDetails.RunID = common.GetRunID()

		//Getting node memory details
		memoryCapacity, memoryAllocatable, err := GetNodeMemoryDetails(appNode, clients)
		if err != nil {
			return errors.Errorf("Unable to get the node memory details, err: %v", err)
		}

		// Get the total memory percentage wrt allocatable memory
		experimentsDetails.MemoryPercentage = CalculateMemoryPercentage(experimentsDetails, clients, memoryCapacity, memoryAllocatable)

		// Creating the helper pod to perform node memory hog
		err = CreateHelperPod(experimentsDetails, appNode, clients, labelSuffix)
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
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod failed due to, err: %v", err)
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

// CalculateMemoryPercentage will calculate the memory percentage under chaos wrt allocatable memory
func CalculateMemoryPercentage(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, memoryCapacity, memoryAllocatable int) int {

	var totalMemoryPercentage int

	//Getting the total memory under chaos
	memoryForChaos := ((float64(experimentsDetails.MemoryPercentage) / 100) * float64(memoryCapacity))

	//Get the percentage of memory under chaos wrt allocatable memory
	totalMemoryPercentage = int((float64(memoryForChaos) / float64(memoryAllocatable)) * 100)

	log.Infof("[Info]: PercentageOfMemoryCapacityToBeUsed: %d, which is %d percent of Allocatable Memory", experimentsDetails.MemoryPercentage, totalMemoryPercentage)

	return totalMemoryPercentage
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, appNode string, clients clients.ClientSets, labelSuffix string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       experimentsDetails.ExperimentName + "-helper-" + labelSuffix,
				"name":                      experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
			},
			Annotations: experimentsDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy: apiv1.RestartPolicyNever,
			NodeName:      appNode,
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
						"1",
						"--vm-bytes",
						strconv.Itoa(experimentsDetails.MemoryPercentage) + "%",
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
