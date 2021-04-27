package lib

import (
	"strconv"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/kubelet-service-kill/types"
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

// PrepareKubeletKill contains prepration steps before chaos injection
func PrepareKubeletKill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var err error
	if experimentsDetails.TargetNode == "" {
		//Select node for kubelet-service-kill
		experimentsDetails.TargetNode, err = common.GetNodeName(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
		if err != nil {
			return err
		}
	}

	log.InfoWithValues("[Info]: Details of node under chaos injection", logrus.Fields{
		"NodeName": experimentsDetails.TargetNode,
	})

	experimentsDetails.RunID = common.GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.TargetNode + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

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

	// Creating the helper pod to perform node memory hog
	err = CreateHelperPod(experimentsDetails, clients, experimentsDetails.TargetNode)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	appLabel := "name=" + experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			return err
		}
	}

	// Checking for the node to be in not-ready state
	log.Info("[Status]: Check for the node to be in NotReady state")
	err = status.CheckNodeNotReadyState(experimentsDetails.TargetNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		return errors.Errorf("application node is not in NotReady state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", experimentsDetails.ChaosDuration+30)

	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+30, experimentsDetails.ExperimentName)
	if err != nil || podStatus == "Failed" {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod failed, err: %v", err)
	}

	// Checking the status of target nodes
	log.Info("[Status]: Getting the status of target nodes")
	err = status.CheckNodeStatus(experimentsDetails.TargetNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		log.Warnf("Target nodes are not in the ready state, you may need to manually recover the node, err: %v", err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = common.DeletePod(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appNodeName string) error {

	privileged := true
	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       experimentsDetails.ExperimentName,
				"name":                      experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
			},
			Annotations: experimentsDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy:    apiv1.RestartPolicyNever,
			ImagePullSecrets: experimentsDetails.ImagePullSecrets,
			NodeName:         appNodeName,
			Volumes: []apiv1.Volume{
				{
					Name: "bus",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: "/var/run",
						},
					},
				},
				{
					Name: "root",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: "/",
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
						"/bin/bash",
					},
					Args: []string{
						"-c",
						"sleep 10 && systemctl stop kubelet && sleep " + strconv.Itoa(experimentsDetails.ChaosDuration) + " && systemctl start kubelet",
					},
					Resources: experimentsDetails.Resources,
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "bus",
							MountPath: "/var/run",
						},
						{
							Name:      "root",
							MountPath: "/node",
						},
					},
					SecurityContext: &apiv1.SecurityContext{
						Privileged: &privileged,
					},
					TTY: true,
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}
