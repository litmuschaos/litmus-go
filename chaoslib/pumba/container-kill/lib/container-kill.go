package lib

import (
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PrepareContainerKill contains the prepration steps before chaos injection
func PrepareContainerKill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	targetPodList, err := common.GetPodList(experimentsDetails.AppNS, experimentsDetails.TargetPod, experimentsDetails.AppLabel, experimentsDetails.PodsAffectedPerc, clients)
	if err != nil {
		return errors.Errorf("Unable to get the target pod list, err: %v", err)
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Get the target container name of the application pod
	if experimentsDetails.TargetContainer == "" {
		experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, targetPodList.Items[0].Name, clients)
		if err != nil {
			return errors.Errorf("Unable to get the target container name, err: %v", err)
		}
	}

	// Get Chaos Pod Annotation
	experimentsDetails.Annotations, err = common.GetChaosPodAnnotation(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
	if err != nil {
		return errors.Errorf("unable to get annotations, err: %v", err)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on target pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	//GetRestartCount return the restart count of target container
	restartCountBefore := GetRestartCount(targetPodList, experimentsDetails.TargetContainer)
	log.Infof("restartCount of target containers before chaos injection: %v", restartCountBefore)

	// creating the helper pod to perform container kill chaos
	for _, pod := range targetPodList.Items {

		runID := common.GetRunID()

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":       pod.Name,
			"NodeName":      pod.Spec.NodeName,
			"ContainerName": experimentsDetails.TargetContainer,
		})

		err = CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}
	}

	//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	log.Infof("[Wait]: Waiting for the %vs chaos duration", experimentsDetails.ChaosDuration)
	common.WaitForDuration(experimentsDetails.ChaosDuration)

	// It will verify that the restart count of container should increase after chaos injection
	err = VerifyRestartCount(experimentsDetails, targetPodList, clients, restartCountBefore)
	if err != nil {
		return errors.Errorf("Target container is not restarted , err: %v", err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = common.DeleteAllPod("app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
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

//GetTargetContainer will fetch the container name from application pod
//This container will be used as target container
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", err
	}

	return pod.Spec.Containers[0].Name, nil
}

//GetRestartCount return the restart count of target container
func GetRestartCount(targetPodList apiv1.PodList, containerName string) []int {
	restartCount := []int{}
	for _, pod := range targetPodList.Items {
		for _, container := range pod.Status.ContainerStatuses {
			if container.Name == containerName {
				restartCount = append(restartCount, int(container.RestartCount))
				break
			}
		}
	}
	return restartCount
}

//VerifyRestartCount verify the restart count of target container that it is restarted or not after chaos injection
// the restart count of container should increase after chaos injection
func VerifyRestartCount(experimentsDetails *experimentTypes.ExperimentDetails, podList apiv1.PodList, clients clients.ClientSets, restartCountBefore []int) error {

	restartCountAfter := []int{}
	err := retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			for index := range podList.Items {
				pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(podList.Items[index].Name, v1.GetOptions{})
				if err != nil {
					return err
				}
				for _, container := range pod.Status.ContainerStatuses {
					if container.Name == experimentsDetails.TargetContainer {
						restartCountAfter = append(restartCountAfter, int(container.RestartCount))
						break
					}
				}
			}
			return nil
		})

	if err != nil {
		return err
	}

	for index := range restartCountBefore {
		// it will fail if restart count won't increase
		if restartCountAfter[index] <= restartCountBefore[index] {
			return errors.Errorf("Target container is not restarted")
		}
	}

	log.Infof("restartCount of target container after chaos injection: %v", restartCountAfter)

	return nil

}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName, appNodeName, runID string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      experimentsDetails.ExperimentName + "-helper",
				"name":     experimentsDetails.ExperimentName + "-" + runID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
			Annotations: experimentsDetails.Annotations,
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
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullAlways,
					Command: []string{
						"pumba",
					},
					Args: []string{
						"--random",
						"--interval",
						strconv.Itoa(experimentsDetails.ChaosInterval) + "s",
						"kill",
						"--signal",
						"SIGKILL",
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
