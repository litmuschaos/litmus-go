package lib

import (
	"strconv"
	"strings"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//PreparePodNetworkChaos contains the prepration steps before chaos injection
func PreparePodNetworkChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	targetPodList, err := common.GetPodList(experimentsDetails.AppNS, experimentsDetails.TargetPod, experimentsDetails.AppLabel, experimentsDetails.PodsAffectedPerc, clients)
	if err != nil {
		return errors.Errorf("Unable to get the target pod list due to, err: %v", err)
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	for _, pod := range targetPodList.Items {

		//Get the target container name of the application pod
		if experimentsDetails.TargetContainer == "" {
			experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, pod.Name, clients)
			if err != nil {
				return errors.Errorf("Unable to get the target container name due to, err: %v", err)
			}
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":       pod.Name,
			"NodeName":      pod.Spec.NodeName,
			"ContainerName": experimentsDetails.TargetContainer,
		})

		experimentsDetails.RunID = common.GetRunID()

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		// Creating the helper pod to perform network chaos
		err = CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName)
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
		log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration))

		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, clients, experimentsDetails.ChaosDuration+30, "pumba")
		if err != nil || podStatus == "Failed" {
			return errors.Errorf("helper pod failed due to, err: %v", err)
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		err = common.DeletePod(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
		if err != nil {
			return errors.Errorf("Unable to delete the helper pod, err: %v", err)
		}
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//GetTargetContainer will fetch the container name from application pod
//This container will be used as target container
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "Fail to get the application pod status, due to:%v", err)
	}

	return pod.Spec.Containers[0].Name, nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName, appNodeName string) error {

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
					Args: GetContainerArguments(experimentsDetails, appName),
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

// GetContainerArguments derives the args for the pumba pod
func GetContainerArguments(experimentsDetails *experimentTypes.ExperimentDetails, appName string) []string {
	baseArgs := []string{
		"netem",
		"--tc-image",
		experimentsDetails.TCImage,
		"--interface",
		experimentsDetails.NetworkInterface,
		"--duration",
		strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
	}

	args := baseArgs
	args = AddTargetIpsArgs(experimentsDetails, args)
	if experimentsDetails.ExperimentName == "pod-network-duplication" {
		args = append(args, "duplicate", "--percent", strconv.Itoa(experimentsDetails.NetworkPacketDuplicationPercentage))
	} else if experimentsDetails.ExperimentName == "pod-network-latency" {
		args = append(args, "delay", "--time", strconv.Itoa(experimentsDetails.NetworkLatency))
	} else if experimentsDetails.ExperimentName == "pod-network-loss" {
		args = append(args, "loss", "--percent", strconv.Itoa(experimentsDetails.NetworkPacketLossPercentage))
	} else if experimentsDetails.ExperimentName == "pod-network-corruption" {
		args = append(args, "corrupt", "--percent", strconv.Itoa(experimentsDetails.NetworkPacketCorruptionPercentage))
	}
	args = append(args, "re2:k8s_"+experimentsDetails.TargetContainer+"_"+appName)
	log.Infof("Arguments for running %v are %v", experimentsDetails.ExperimentName, args)
	return args
}

// AddTargetIpsArgs inserts a comma-separated list of targetIPs (if provided by the user) into the pumba command/args
func AddTargetIpsArgs(experimentsDetails *experimentTypes.ExperimentDetails, args []string) []string {
	if experimentsDetails.TargetIPs == "" {
		return args
	}
	ips := strings.Split(experimentsDetails.TargetIPs, ",")
	for i := range ips {
		args = append(args, "--target", strings.TrimSpace(ips[i]))
	}
	return args
}
