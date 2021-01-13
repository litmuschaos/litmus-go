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
	targetPodList, err := common.GetPodList(experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
	if err != nil {
		return err
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

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on target pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	if experimentsDetails.Sequence == "serial" {
		if err = InjectChaosInSerialMode(experimentsDetails, targetPodList, clients, chaosDetails); err != nil {
			return err
		}
	} else {
		if err = InjectChaosInParallelMode(experimentsDetails, targetPodList, clients, chaosDetails); err != nil {
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

// InjectChaosInSerialMode kill the container of all target application serially (one by one)
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	// creating the helper pod to perform container kill chaos
	for _, pod := range targetPodList.Items {

		//GetRestartCount return the restart count of target container
		restartCountBefore := GetRestartCount(pod, experimentsDetails.TargetContainer)
		log.Infof("restartCount of target container before chaos injection: %v", restartCountBefore)

		runID := common.GetRunID()

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":       pod.Name,
			"NodeName":      pod.Spec.NodeName,
			"ContainerName": experimentsDetails.TargetContainer,
		})

		err := CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}

		//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
		log.Info("[Status]: Checking the status of the helper pod")
		err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("helper pod is not in running state, err: %v", err)
		}

		log.Infof("[Wait]: Waiting for the %vs chaos duration", experimentsDetails.ChaosDuration)
		common.WaitForDuration(experimentsDetails.ChaosDuration)

		// It will verify that the restart count of container should increase after chaos injection
		err = VerifyRestartCount(experimentsDetails, pod, clients, restartCountBefore)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("Target container is not restarted, err: %v", err)
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		err = common.DeletePod(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
		if err != nil {
			return errors.Errorf("Unable to delete the helper pod, err: %v", err)
		}
	}

	return nil
}

// InjectChaosInParallelMode kill the container of all target application in parallel mode (all at once)
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	//GetRestartCount return the restart count of target container
	restartCountBefore := GetRestartCountAll(targetPodList, experimentsDetails.TargetContainer)
	log.Infof("restartCount of target containers before chaos injection: %v", restartCountBefore)

	// creating the helper pod to perform container kill chaos
	for _, pod := range targetPodList.Items {

		runID := common.GetRunID()

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":       pod.Name,
			"NodeName":      pod.Spec.NodeName,
			"ContainerName": experimentsDetails.TargetContainer,
		})

		err := CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}
	}

	//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	err := status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	log.Infof("[Wait]: Waiting for the %vs chaos duration", experimentsDetails.ChaosDuration)
	common.WaitForDuration(experimentsDetails.ChaosDuration)

	// It will verify that the restart count of container should increase after chaos injection
	err = VerifyRestartCountAll(experimentsDetails, targetPodList, clients, restartCountBefore)
	if err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
		return errors.Errorf("Target container is not restarted , err: %v", err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = common.DeleteAllPod("app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
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
func GetRestartCount(targetPod apiv1.Pod, containerName string) int {
	restartCount := 0
	for _, container := range targetPod.Status.ContainerStatuses {
		if container.Name == containerName {
			restartCount = int(container.RestartCount)
			break
		}
	}
	return restartCount
}

//GetRestartCountAll return the restart count of all target container
func GetRestartCountAll(targetPodList apiv1.PodList, containerName string) []int {
	restartCount := []int{}
	for _, pod := range targetPodList.Items {

		restartCount = append(restartCount, GetRestartCount(pod, containerName))

	}

	return restartCount
}

//VerifyRestartCount verify the restart count of target container that it is restarted or not after chaos injection
// the restart count of container should increase after chaos injection
func VerifyRestartCount(experimentsDetails *experimentTypes.ExperimentDetails, pod apiv1.Pod, clients clients.ClientSets, restartCountBefore int) error {

	restartCountAfter := 0
	err := retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(pod.Name, v1.GetOptions{})
			if err != nil {
				return err
			}
			for _, container := range pod.Status.ContainerStatuses {
				if container.Name == experimentsDetails.TargetContainer {
					restartCountAfter = int(container.RestartCount)
					break
				}
			}
			return nil
		})

	if err != nil {
		return err
	}

	// it will fail if restart count won't increase
	if restartCountAfter <= restartCountBefore {
		return errors.Errorf("Target container is not restarted")
	}

	log.Infof("restartCount of target container after chaos injection: %v", restartCountAfter)

	return nil

}

//VerifyRestartCountAll verify the restart count of all the target container that it is restarted or not after chaos injection
// the restart count of container should increase after chaos injection
func VerifyRestartCountAll(experimentsDetails *experimentTypes.ExperimentDetails, podList apiv1.PodList, clients clients.ClientSets, restartCountBefore []int) error {

	for index, pod := range podList.Items {

		if err := VerifyRestartCount(experimentsDetails, pod, clients, restartCountBefore[index]); err != nil {
			return err
		}
	}
	return nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName, appNodeName, runID string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       experimentsDetails.ExperimentName + "-helper",
				"name":                      experimentsDetails.ExperimentName + "-" + runID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
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
							Path: experimentsDetails.SocketPath,
						},
					},
				},
			},
			InitContainers: []apiv1.Container{
				{
					Name:            "setup-" + experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"/bin/bash",
						"-c",
						"sudo chmod 777 " + experimentsDetails.SocketPath,
					},
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "dockersocket",
							MountPath: experimentsDetails.SocketPath,
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
					Resources: experimentsDetails.Resources,
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "dockersocket",
							MountPath: experimentsDetails.SocketPath,
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}
