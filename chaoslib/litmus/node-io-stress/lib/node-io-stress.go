package lib

import (
	"strconv"
	"strings"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-io-stress/types"
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

// PrepareNodeIOStress contains prepration steps before chaos injection
func PrepareNodeIOStress(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Select node for node-io-stress
	targetNodeList, err := common.GetNodeList(experimentsDetails.TargetNodes, experimentsDetails.NodeLabel, experimentsDetails.NodesAffectedPerc, clients)
	if err != nil {
		return err
	}
	log.InfoWithValues("[Info]: Details of Nodes under chaos injection", logrus.Fields{
		"No. Of Nodes": len(targetNodeList),
		"Node Names":   targetNodeList,
	})

	if experimentsDetails.EngineName != "" {
		if err := setHelperData(experimentsDetails, clients); err != nil {
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

// injectChaosInSerialMode stress the io of all the target nodes serially (one by one)
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
			"NodeName":                        appNode,
			"FilesystemUtilizationPercentage": experimentsDetails.FilesystemUtilizationPercentage,
			"NumberOfWorkers":                 experimentsDetails.NumberOfWorkers,
		})

		experimentsDetails.RunID = common.GetRunID()

		// Creating the helper pod to perform node io stress
		if err := createHelperPod(experimentsDetails, appNode, clients, labelSuffix); err != nil {
			return errors.Errorf("unable to create the helper pod, err: %v", err)
		}

		appLabel := "name=" + experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID

		//Checking the status of helper pod
		log.Info("[Status]: Checking the status of the helper pod")
		if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod is not in running state, err: %v", err)
		}
		common.SetTargets(appNode, "injected", "node", chaosDetails)

		log.Info("[Wait]: Waiting till the completion of the helper pod")
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.ExperimentName)
		common.SetTargets(appNode, "reverted", "node", chaosDetails)
		if err != nil || podStatus == "Failed" {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod failed due to, err: %v", err)
		}

		// Checking the status of target nodes
		log.Info("[Status]: Getting the status of target nodes")
		if err = status.CheckNodeStatus(appNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
			log.Warnf("Target nodes are not in the ready state, you may need to manually recover the node, err: %v", err)
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		if err = common.DeletePod(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			return errors.Errorf("unable to delete the helper pod, err: %v", err)
		}
	}
	return nil
}

// injectChaosInParallelMode stress the io of all the target nodes in parallel mode (all at once)
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
			"NodeName":                        appNode,
			"FilesystemUtilizationPercentage": experimentsDetails.FilesystemUtilizationPercentage,
			"NumberOfWorkers":                 experimentsDetails.NumberOfWorkers,
		})

		experimentsDetails.RunID = common.GetRunID()

		// Creating the helper pod to perform node io stress
		if err := createHelperPod(experimentsDetails, appNode, clients, labelSuffix); err != nil {
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
		common.SetTargets(appNode, "injected", "node", chaosDetails)
	}

	log.Info("[Wait]: Waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.ExperimentName)
	for _, appNode := range targetNodeList {
		common.SetTargets(appNode, "reverted", "node", chaosDetails)
	}
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod failed due to, err: %v", err)
	}

	for _, appNode := range targetNodeList {

		// Checking the status of application node
		log.Info("[Status]: Getting the status of application node")
		if err = status.CheckNodeStatus(appNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			log.Warn("Application node is not in the ready state, you may need to manually recover the node")
		}
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	if err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return errors.Errorf("unable to delete the helper pod, err: %v", err)
	}

	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, appNode string, clients clients.ClientSets, labelSuffix string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      experimentsDetails.ExperimentName + "-helper-" + labelSuffix,
				"name":     experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID,
				"chaosUID": string(experimentsDetails.ChaosUID),
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
					Args:      getContainerArguments(experimentsDetails),
					Resources: experimentsDetails.Resources,
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}

// getContainerArguments derives the args for the pumba stress helper pod
func getContainerArguments(experimentsDetails *experimentTypes.ExperimentDetails) []string {

	var hddbytes string
	if experimentsDetails.FilesystemUtilizationBytes == 0 {
		if experimentsDetails.FilesystemUtilizationPercentage == 0 {
			hddbytes = "10%"
			log.Info("Neither of FilesystemUtilizationPercentage or FilesystemUtilizationBytes provided, proceeding with a default FilesystemUtilizationPercentage value of 10%")
		} else {
			hddbytes = strconv.Itoa(experimentsDetails.FilesystemUtilizationPercentage) + "%"
		}
	} else {
		if experimentsDetails.FilesystemUtilizationPercentage == 0 {
			hddbytes = strconv.Itoa(experimentsDetails.FilesystemUtilizationBytes) + "G"
		} else {
			hddbytes = strconv.Itoa(experimentsDetails.FilesystemUtilizationPercentage) + "%"
			log.Warn("Both FsUtilPercentage & FsUtilBytes provided as inputs, using the FsUtilPercentage value to proceed with stress exp")
		}
	}

	stressArgs := []string{
		"--cpu",
		strconv.Itoa(experimentsDetails.CPU),
		"--vm",
		strconv.Itoa(experimentsDetails.VMWorkers),
		"--io",
		strconv.Itoa(experimentsDetails.NumberOfWorkers),
		"--hdd",
		strconv.Itoa(experimentsDetails.NumberOfWorkers),
		"--hdd-bytes",
		hddbytes,
		"--timeout",
		strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
		"--temp-path",
		"/tmp",
	}
	return stressArgs
}

// setHelperData derive the data from experiment pod and sets into experimentDetails struct
// which can be used to create helper pod
func setHelperData(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	// Get Chaos Pod Annotation
	var err error
	experimentsDetails.Annotations, err = common.GetChaosPodAnnotation(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
	if err != nil {
		return errors.Errorf("unable to get annotations, err: %v", err)
	}
	// Get Resource Requirements
	experimentsDetails.Resources, err = common.GetChaosPodResourceRequirements(experimentsDetails.ChaosPodName, experimentsDetails.ExperimentName, experimentsDetails.ChaosNamespace, clients)
	if err != nil {
		return errors.Errorf("unable to get resource requirements, err: %v", err)
	}
	// Get ImagePullSecrets
	experimentsDetails.ImagePullSecrets, err = common.GetImagePullSecrets(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
	if err != nil {
		return errors.Errorf("unable to get imagePullSecrets, err: %v", err)
	}
	return nil
}
