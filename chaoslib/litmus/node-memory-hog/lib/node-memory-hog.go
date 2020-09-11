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

	//Select the node name
	appNodeName, err := common.GetNodeName(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		return errors.Errorf("Unable to get the node name due to, err: %v", err)
	}

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"NodeName":             appNodeName,
		"MemoryHog Percentage": experimentsDetails.MemoryPercentage,
	})

	experimentsDetails.RunID = common.GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNodeName + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	//Getting node memory details
	memoryCapacity, memoryAllocatable, err := GetNodeMemoryDetails(appNodeName, clients)
	if err != nil {
		return errors.Errorf("Unable to get the node memory details, err: %v", err)
	}

	// Get the total memory percentage wrt allocatable memory
	experimentsDetails.MemoryPercentage = CalculateMemoryPercentage(experimentsDetails, clients, memoryCapacity, memoryAllocatable)

	// Get Chaos Pod Annotation
	experimentsDetails.Annotations, err = common.GetChaosPodAnnotation(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
	if err != nil {
		return errors.Errorf("unable to get annotation, due to %v", err)
	}

	// Creating the helper pod to perform node memory hog
	err = CreateHelperPod(experimentsDetails, clients, appNodeName)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration+30))

	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, clients, experimentsDetails.ChaosDuration+30, experimentsDetails.ExperimentName)
	if err != nil || podStatus == "Failed" {
		return errors.Errorf("helper pod failed due to, err: %v", err)
	}

	// Checking the status of application node
	log.Info("[Status]: Getting the status of application node")
	err = status.CheckNodeStatus(appNodeName, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		log.Warn("Application node is not in the ready state, you may need to manually recover the node")
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = common.DeletePod(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// GetNodeMemoryDetails will return the total memory capacity and memory allocatable of an application node
func GetNodeMemoryDetails(appNodeName string, clients clients.ClientSets) (int, int, error) {

	nodeDetails, err := clients.KubeClient.CoreV1().Nodes().Get(appNodeName, v1.GetOptions{})
	if err != nil {
		return 0, 0, errors.Errorf("Fail to get nodesDetails, due to %v", err)
	}

	memoryCapacity := int(nodeDetails.Status.Capacity.Memory().Value())
	memoryAllocatable := int(nodeDetails.Status.Allocatable.Memory().Value())

	if memoryCapacity == 0 || memoryAllocatable == 0 {
		return memoryCapacity, memoryAllocatable, errors.Errorf("Fail to get memory details of the application node")
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
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appNodeName string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      experimentsDetails.ExperimentName,
				"name":     experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
			Annotations: experimentsDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy: apiv1.RestartPolicyNever,
			NodeName:      appNodeName,
			Containers: []apiv1.Container{
				{
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullAlways,
					Command: []string{
						"/stress-ng",
					},
					Args: []string{
						"--vm",
						"1",
						"--vm-bytes",
						strconv.Itoa(experimentsDetails.MemoryPercentage) + "%",
						"--timeout",
						strconv.Itoa(experimentsDetails.ChaosDuration),
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}
