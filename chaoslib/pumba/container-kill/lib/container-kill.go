package lib

import (
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
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
		return errors.Errorf("Unable to get the target pod list due to, err: %v", err)
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Getting the iteration count for the container-kill
	GetIterations(experimentsDetails)

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

loop:
	for count := 0; count < experimentsDetails.Iterations; count++ {
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

			// generating a unique string which can be appended with the helper pod name & labels for the uniquely identification
			experimentsDetails.RunID = common.GetRunID()

			//GetRestartCount return the restart count of target container
			restartCountBefore, err := GetRestartCount(experimentsDetails, pod.Name, clients)
			if err != nil {
				return err
			}
			log.Infof("restartCount of target container before chaos injection: %v", strconv.Itoa(restartCountBefore))

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			// creating the helper pod to perform container kill chaos
			if err = CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName); err != nil {
				return errors.Errorf("Unable to create the helper pod due to, err: %v", err)
			}

			//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
			log.Info("[Status]: Checking the status of the helper pod")
			err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
			if err != nil {
				return errors.Errorf("helper pod is not in running state, err: %v", err)
			}

			// It will verify that the restart count of container should increase after chaos injection
			err = VerifyRestartCount(experimentsDetails, pod.Name, clients, restartCountBefore)
			if err != nil {
				return errors.Errorf("Target container is not restarted , err: %v", err)
			}

			//Deleting the helper pod
			log.Info("[Cleanup]: Deleting the helper pod")
			err = common.DeletePod(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
			if err != nil {
				return errors.Errorf("Unable to delete the helper pod, err: %v", err)
			}

		}
		log.Infof("[Wait]: Waiting for the chaos interval of %v seconds", experimentsDetails.ChaosInterval)
		common.WaitForDuration(experimentsDetails.ChaosInterval)

		endTime = time.After(timeDelay)
		select {
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			break loop
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

//GetRestartCount return the restart count of target container
func GetRestartCount(experimentsDetails *experimentTypes.ExperimentDetails, podName string, clients clients.ClientSets) (int, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(podName, v1.GetOptions{})
	if err != nil {
		return 0, err
	}
	restartCount := 0
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == experimentsDetails.TargetContainer {
			restartCount = int(container.RestartCount)
			break
		}
	}
	return restartCount, nil
}

//VerifyRestartCount verify the restart count of target container that it is restarted or not after chaos injection
// the restart count of container should increase after chaos injection
func VerifyRestartCount(experimentsDetails *experimentTypes.ExperimentDetails, podName string, clients clients.ClientSets, restartCountBefore int) error {

	restartCountAfter := 0
	err := retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(podName, v1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to get the application pod, err: %v", err)
			}
			for _, container := range pod.Status.ContainerStatuses {
				if container.Name == experimentsDetails.TargetContainer {
					restartCountAfter = int(container.RestartCount)
					break
				}
			}
			// it will fail if restart count won't increase
			if restartCountAfter <= restartCountBefore {
				return errors.Errorf("Target container is not restarted")
			}
			return nil
		})

	log.Infof("restartCount of target container after chaos injection: %v", strconv.Itoa(restartCountAfter))

	return err

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

//GetIterations derive the iterations value from given parameters
func GetIterations(experimentsDetails *experimentTypes.ExperimentDetails) {
	var Iterations int
	if experimentsDetails.ChaosInterval != 0 {
		Iterations = experimentsDetails.ChaosDuration / experimentsDetails.ChaosInterval
	}
	experimentsDetails.Iterations = math.Maximum(Iterations, 1)

}
