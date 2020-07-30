package network_chaos

import (
	"math/rand"
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//PreparePodNetworkChaos contains the prepration steps before chaos injection
func PreparePodNetworkChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Select application pod for network chaos
	appName, appNodeName, err := GetApplicationPod(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the application name and application nodename due to, err: %v", err)
	}

	//Get the target container name of the application pod
	if experimentsDetails.TargetContainer == "" {
		experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, appName, clients)
		if err != nil {
			return errors.Errorf("Unable to get the target container name due to, err: %v", err)
		}
	}

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"PodName":       appName,
		"NodeName":      appNodeName,
		"ContainerName": experimentsDetails.TargetContainer,
	})

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}

	experimentsDetails.RunID = GetRunID()

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appName + " pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// Creating the helper pod to perform network chaos
	err = CreateHelperPod(experimentsDetails, clients, appName, appNodeName)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name=pumba-netem-"+experimentsDetails.RunID, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration))

	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "name=pumba-netem-"+experimentsDetails.RunID, clients, experimentsDetails.ChaosDuration+30, "pumba")
	if err != nil || podStatus == "Failed" {
		return errors.Errorf("helper pod failed due to, err: %v", err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = DeleteHelperPod(experimentsDetails, clients, experimentsDetails.RunID)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
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

//GetTargetContainer will fetch the conatiner name from application pod
//This container will be used as target container
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "Fail to get the application pod status, due to:%v", err)
	}

	return pod.Spec.Containers[0].Name, nil
}

//waitForRampTime waits for the given ramp time duration (in seconds)
func waitForRampTime(experimentsDetails *experimentTypes.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.RampTime) * time.Second)
}

// GetRunID generate a random string
func GetRunID() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")
	runID := make([]rune, 6)
	for i := range runID {
		runID[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(runID)
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName, appNodeName string) error {

	command, keyAttibute, valueAttribute := GetNetworkChaosCommands(experimentsDetails)

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pumba-netem-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      "pumba",
				"name":     "pumba-netem-" + experimentsDetails.RunID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
		},
		Spec: apiv1.PodSpec{
			RestartPolicy: apiv1.RestartPolicyNever,
			NodeName:      appNodeName,
			Volumes: []apiv1.Volume{
				{
					Name: "dockersocket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: "/var/run/docker.sock",
						},
					},
				},
			},
			Containers: []apiv1.Container{
				{
					Name:            "pumba",
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullAlways,
					Command: []string{
						"pumba",
					},
					Args: []string{
						"netem",
						"--tc-image",
						experimentsDetails.TCImage,
						"--interface",
						experimentsDetails.NetworkInterface,
						"--duration",
						strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
						command,
						keyAttibute,
						valueAttribute,
						"re2:k8s_" + experimentsDetails.TargetContainer + "_" + appName,
					},
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "dockersocket",
							MountPath: "/var/run/docker.sock",
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}

//DeleteHelperPod delete the helper pod
func DeleteHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, runID string) error {

	err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Delete("pumba-netem-"+runID, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	err = retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: "name=pumba-netem-" + runID})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Helper Pod is not deleted yet, err: %v", err)
			}
			return nil
		})

	return err
}

// GetNetworkChaosCommands derive the commands for the pumba pod
func GetNetworkChaosCommands(experimentsDetails *experimentTypes.ExperimentDetails) (string, string, string) {

	var command, keyAttibute, valueAttribute string
	if experimentsDetails.ExperimentName == "pod-network-duplication" {
		command = "duplicate"
		keyAttibute = "--percent"
		valueAttribute = strconv.Itoa(experimentsDetails.NetworkPacketDuplicationPercentage)
	} else if experimentsDetails.ExperimentName == "pod-network-latency" {
		command = "delay"
		keyAttibute = "--time"
		valueAttribute = strconv.Itoa(experimentsDetails.NetworkLatency)
	} else if experimentsDetails.ExperimentName == "pod-network-loss" {
		command = "loss"
		keyAttibute = "--percent"
		valueAttribute = strconv.Itoa(experimentsDetails.NetworkPacketLossPercentage)
	} else if experimentsDetails.ExperimentName == "pod-network-corruption" {
		command = "corrupt"
		keyAttibute = "--percent"
		valueAttribute = strconv.Itoa(experimentsDetails.NetworkPacketCorruptionPercentage)
	}
	return command, keyAttibute, valueAttribute
}
