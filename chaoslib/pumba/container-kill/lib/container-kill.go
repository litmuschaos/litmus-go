package lib

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/container-kill/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
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
	if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail.Label == "" {
		return errors.Errorf("please provide one of the appLabel or TARGET_PODS")
	}
	//Setup the tunables if provided in range
	litmusLIB.SetChaosTunables(experimentsDetails)

	log.InfoWithValues("[Info]: The tunables are:", logrus.Fields{
		"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
		"Sequence":         experimentsDetails.Sequence,
	})
	podsAffectedPerc, _ := strconv.Atoi(experimentsDetails.PodsAffectedPerc)
	targetPodList, err := common.GetPodList(experimentsDetails.TargetPods, podsAffectedPerc, clients, chaosDetails)
	if err != nil {
		return err
	}

	podNames := []string{}
	for _, pod := range targetPodList.Items {
		podNames = append(podNames, pod.Name)
	}
	log.Infof("Target pods list for chaos, %v", podNames)

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		if err := common.SetHelperData(chaosDetails, experimentsDetails.SetHelperData, clients); err != nil {
			return err
		}
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on target pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	experimentsDetails.IsTargetContainerProvided = (experimentsDetails.TargetContainer != "")
	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(experimentsDetails, targetPodList, clients, chaosDetails, resultDetails, eventsDetails); err != nil {
			return err
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, targetPodList, clients, chaosDetails, resultDetails, eventsDetails); err != nil {
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

// injectChaosInSerialMode kill the container of all target application serially (one by one)
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	var err error
	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// creating the helper pod to perform container kill chaos
	for _, pod := range targetPodList.Items {

		//GetRestartCount return the restart count of target container
		restartCountBefore := getRestartCount(pod, experimentsDetails.TargetContainer)
		log.Infof("restartCount of target container before chaos injection: %v", restartCountBefore)

		runID := common.GetRunID()

		//Get the target container name of the application pod
		if !experimentsDetails.IsTargetContainerProvided {
			experimentsDetails.TargetContainer, err = common.GetTargetContainer(experimentsDetails.AppNS, pod.Name, clients)
			if err != nil {
				return errors.Errorf("unable to get the target container name, err: %v", err)
			}
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"Target Pod":       pod.Name,
			"NodeName":         pod.Spec.NodeName,
			"Target Container": experimentsDetails.TargetContainer,
		})

		if err := createHelperPod(experimentsDetails, clients, chaosDetails, pod.Name, pod.Spec.NodeName, runID); err != nil {
			return errors.Errorf("unable to create the helper pod, err: %v", err)
		}

		common.SetTargets(pod.Name, "targeted", "pod", chaosDetails)

		appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, runID)

		//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
		log.Info("[Status]: Checking the status of the helper pod")
		if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod is not in running state, err: %v", err)
		}

		log.Infof("[Wait]: Waiting for the %vs chaos duration", experimentsDetails.ChaosDuration)
		common.WaitForDuration(experimentsDetails.ChaosDuration)

		// It will verify that the restart count of container should increase after chaos injection
		if err := verifyRestartCount(experimentsDetails, pod, clients, restartCountBefore); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			return errors.Errorf("target container is not restarted, err: %v", err)
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		if err := common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			return errors.Errorf("unable to delete the helper pod, err: %v", err)
		}
	}

	return nil
}

// injectChaosInParallelMode kill the container of all target application in parallel mode (all at once)
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	var err error
	//GetRestartCount return the restart count of target container
	restartCountBefore := getRestartCountAll(targetPodList, experimentsDetails.TargetContainer)
	log.Infof("restartCount of target containers before chaos injection: %v", restartCountBefore)

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	runID := common.GetRunID()

	// creating the helper pod to perform container kill chaos
	for _, pod := range targetPodList.Items {

		//Get the target container name of the application pod
		if !experimentsDetails.IsTargetContainerProvided {
			experimentsDetails.TargetContainer, err = common.GetTargetContainer(experimentsDetails.AppNS, pod.Name, clients)
			if err != nil {
				return errors.Errorf("unable to get the target container name, err: %v", err)
			}
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"Target Pod":       pod.Name,
			"NodeName":         pod.Spec.NodeName,
			"Target Container": experimentsDetails.TargetContainer,
		})

		if err := createHelperPod(experimentsDetails, clients, chaosDetails, pod.Name, pod.Spec.NodeName, runID); err != nil {
			return errors.Errorf("unable to create the helper pod, err: %v", err)
		}
		common.SetTargets(pod.Name, "targeted", "pod", chaosDetails)
	}

	appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, runID)

	//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	log.Infof("[Wait]: Waiting for the %vs chaos duration", experimentsDetails.ChaosDuration)
	common.WaitForDuration(experimentsDetails.ChaosDuration)

	// It will verify that the restart count of container should increase after chaos injection
	if err := verifyRestartCountAll(experimentsDetails, targetPodList, clients, restartCountBefore); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("target container is not restarted , err: %v", err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	if err := common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return errors.Errorf("unable to delete the helper pod, err: %v", err)
	}

	return nil
}

//getRestartCount return the restart count of target container
func getRestartCount(targetPod apiv1.Pod, containerName string) int {
	restartCount := 0
	for _, container := range targetPod.Status.ContainerStatuses {
		if container.Name == containerName {
			restartCount = int(container.RestartCount)
			break
		}
	}
	return restartCount
}

//getRestartCountAll return the restart count of all target container
func getRestartCountAll(targetPodList apiv1.PodList, containerName string) []int {
	restartCount := []int{}
	for _, pod := range targetPodList.Items {
		restartCount = append(restartCount, getRestartCount(pod, containerName))
	}

	return restartCount
}

//verifyRestartCount verify the restart count of target container that it is restarted or not after chaos injection
// the restart count of container should increase after chaos injection
func verifyRestartCount(experimentsDetails *experimentTypes.ExperimentDetails, pod apiv1.Pod, clients clients.ClientSets, restartCountBefore int) error {

	restartCountAfter := 0
	err := retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(context.Background(), pod.Name, v1.GetOptions{})
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
		return errors.Errorf("target container is not restarted")
	}

	log.Infof("restartCount of target container after chaos injection: %v", restartCountAfter)

	return nil
}

//verifyRestartCountAll verify the restart count of all the target container that it is restarted or not after chaos injection
// the restart count of container should increase after chaos injection
func verifyRestartCountAll(experimentsDetails *experimentTypes.ExperimentDetails, podList apiv1.PodList, clients clients.ClientSets, restartCountBefore []int) error {

	for index, pod := range podList.Items {

		if err := verifyRestartCount(experimentsDetails, pod, clients, restartCountBefore[index]); err != nil {
			return err
		}
	}
	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, appName, appNodeName, runID string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: experimentsDetails.ExperimentName + "-helper-",
			Namespace:    experimentsDetails.ChaosNamespace,
			Labels:       common.GetHelperLabels(chaosDetails.Labels, runID, experimentsDetails.ExperimentName),
			Annotations:  chaosDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy:    apiv1.RestartPolicyNever,
			ImagePullSecrets: chaosDetails.ImagePullSecrets,
			NodeName:         appNodeName,
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
			Containers: []apiv1.Container{
				{
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"sudo",
						"-E",
					},
					Args: []string{
						"pumba",
						"--random",
						"--interval",
						strconv.Itoa(experimentsDetails.ChaosInterval) + "s",
						"kill",
						"--signal",
						experimentsDetails.Signal,
						"re2:k8s_" + experimentsDetails.TargetContainer + "_" + appName,
					},
					Env: []apiv1.EnvVar{
						{
							Name:  "DOCKER_HOST",
							Value: "unix://" + experimentsDetails.SocketPath,
						},
					},
					Resources: chaosDetails.Resources,
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

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(context.Background(), helperPod, v1.CreateOptions{})
	return err
}
