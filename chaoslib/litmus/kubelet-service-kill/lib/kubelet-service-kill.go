package lib

import (
	"math/rand"
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/kubelet-service-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
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

	if experimentsDetails.AppNode == "" {
		//Select node for kubelet-service-kill
		_, appNodeName, err := GetApplicationPod(experimentsDetails, clients)
		if err != nil {
			return errors.Errorf("Unable to get the application nodename due to, err: %v", err)
		}

		experimentsDetails.AppNode = appNodeName
	}

	log.InfoWithValues("[Info]: Details of node under chaos injection", logrus.Fields{
		"NodeName": experimentsDetails.AppNode,
	})

	experimentsDetails.RunID = common.GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.AppNode + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// Creating the helper pod to perform node memory hog
	err := CreateHelperPod(experimentsDetails, clients, experimentsDetails.AppNode)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Checking for the node to be in not-ready state
	log.Info("[Status]: Check for the node to be in NotReady state")
	err = status.CheckNodeNotReadyState(experimentsDetails.AppNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("application node is not in NotReady state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration+30))

	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, clients, experimentsDetails.ChaosDuration+30, experimentsDetails.ExperimentName)
	if err != nil || podStatus == "Failed" {
		return errors.Errorf("helper pod failed due to, err: %v", err)
	}

	// Checking the status of application node
	log.Info("[Status]: Getting the status of application node")
	err = status.CheckNodeStatus(experimentsDetails.AppNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("application node is not in ready state, err: %v", err)
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

//GetApplicationPod will select a random replica of application pod for chaos
//It will also get the node name of the application pod
func GetApplicationPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, string, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(podList.Items) == 0 {
		return "", "", errors.Wrapf(err, "Fail to get the application pod in %v namespace", experimentsDetails.AppNS)
	}

	rand.Seed(time.Now().Unix())
	randomIndex := rand.Intn(len(podList.Items))
	applicationName := podList.Items[randomIndex].Name
	nodeName := podList.Items[randomIndex].Spec.NodeName

	return applicationName, nodeName, nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appNodeName string) error {

	privileged := true
	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      experimentsDetails.ExperimentName,
				"name":     experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
		},
		Spec: apiv1.PodSpec{
			RestartPolicy: apiv1.RestartPolicyNever,
			NodeName:      appNodeName,
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
					Image:           "ubuntu:16.04",
					ImagePullPolicy: apiv1.PullAlways,
					Command: []string{
						"/bin/bash",
					},
					Args: []string{
						"-c",
						"sleep 10 && systemctl stop kubelet && sleep " + strconv.Itoa(experimentsDetails.ChaosDuration) + " && systemctl start kubelet",
					},
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
