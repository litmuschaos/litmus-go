package lib

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KillContainer kill the random application container
func KillContainer(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, targetPod apiv1.Pod) error {

	execCommandDetails := litmusexec.PodDetails{}

	//Obtain the pod ID of the application pod
	podID, err := GetPodID(experimentsDetails, targetPod, &execCommandDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the pod id %v", err)
	}

	//GetRestartCount return the restart count of the target container
	restartCountBefore, err := GetRestartCount(experimentsDetails, targetPod.Name, clients)
	if err != nil {
		return err
	}

	//Obtain the container ID through Pod
	// this id will be used to select the container for kill
	containerID, err := GetContainerID(experimentsDetails, podID, &execCommandDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the container id, %v", err)
	}

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"PodName":            targetPod.Name,
		"NodeName":           targetPod.Spec.NodeName,
		"RestartCountBefore": restartCountBefore,
	})

	// killing the application container
	StopContainer(containerID, &execCommandDetails, clients)

	//Check the status of restarted container
	err = CheckContainerStatus(experimentsDetails, clients, targetPod.Name)
	if err != nil {
		return errors.Errorf("Application container is not in running state, %v", err)
	}

	// It will verify that the restart count of container should increase after chaos injection
	err = VerifyRestartCount(experimentsDetails, targetPod.Name, clients, restartCountBefore)
	if err != nil {
		return errors.Errorf("Target container is not restarted , err: %v", err)
	}
	return nil

}

//GetPodID derive the pod-id of the application pod
func GetPodID(experimentsDetails *experimentTypes.ExperimentDetails, targetPod apiv1.Pod, execCommandDetails *litmusexec.PodDetails, clients clients.ClientSets) (string, error) {

	// getting the helper pod name, scheduled on the target node
	podName, err := GetHelperPodName(targetPod, clients, experimentsDetails)
	if err != nil {
		return "", err
	}

	// run the commands inside helper pod to get the available pods on the target node
	command := []string{"crictl", "pods"}
	litmusexec.SetExecCommandAttributes(execCommandDetails, podName, experimentsDetails.ExperimentName, experimentsDetails.ChaosNamespace)
	allPods, err := litmusexec.Exec(execCommandDetails, clients, command)
	if err != nil {
		return "", errors.Errorf("Unable to run crictl command, due to err: %v", err)
	}

	// removing all the extra spaces present inside output of crictl command and extract the pod id
	b := []byte(allPods)
	pods := RemoveExtraSpaces(b)
	for i := 0; i < len(pods)-1; i++ {
		attributes := strings.Split(pods[i], " ")
		if attributes[3] == targetPod.Name {
			return attributes[0], nil
		}

	}

	return "", fmt.Errorf("The application pod is unavailable")
}

//GetContainerID  derive the container id of the application container
func GetContainerID(experimentsDetails *experimentTypes.ExperimentDetails, podID string, execCommandDetails *litmusexec.PodDetails, clients clients.ClientSets) (string, error) {

	// run the commands inside helper pod and fetch the result
	command := []string{"crictl", "ps"}
	allContainers, err := litmusexec.Exec(execCommandDetails, clients, command)
	if err != nil {
		return "", errors.Errorf("Unable to run crictl command, due to err: %v", err)
	}

	// removing all the extra spaces present inside output of crictl command and extract out the container id
	b := []byte(allContainers)
	containers := RemoveExtraSpaces(b)
	for i := 0; i < len(containers)-1; i++ {
		attributes := strings.Split(containers[i], " ")
		if attributes[4] == experimentsDetails.TargetContainer && attributes[6] == podID {
			return attributes[0], nil
		}

	}

	return "", fmt.Errorf("The application container is unavailable")

}

//StopContainer kill the application container
func StopContainer(containerID string, execCommandDetails *litmusexec.PodDetails, clients clients.ClientSets) error {

	command := []string{"crictl", "stop", string(containerID)}
	_, err := litmusexec.Exec(execCommandDetails, clients, command)
	return err
}

// CheckContainerStatus checks the status of the application container
func CheckContainerStatus(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName string) error {
	err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to list the pod, err: %v", err)
			}
			for _, container := range pod.Status.ContainerStatuses {
				if container.Ready != true {
					return errors.Errorf("containers are not yet in running state")
				}
				log.InfoWithValues("The running status of container are as follows", logrus.Fields{
					"container": container.Name, "Pod": pod.Name, "Status": pod.Status.Phase})
			}

			return nil
		})

	return err
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
			if restartCountAfter <= restartCountBefore {
				return errors.Errorf("Target container is not restarted")
			}
			return nil
		})

	log.Infof("restartCount of target container after chaos injection: %v", strconv.Itoa(restartCountAfter))

	return err

}

// GetHelperPodName check for the helper pod, which is scheduled on the target node
func GetHelperPodName(targetPod apiv1.Pod, clients clients.ClientSets, experimentsDetails *experimentTypes.ExperimentDetails) (string, error) {

	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: "name=" + experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID})

	if err != nil || len(podList.Items) == 0 {
		return "", errors.Errorf("Unable to list the helper pods due to, err: %v", err)
	}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName == targetPod.Spec.NodeName {
			return pod.Name, nil
		}
	}
	return "", errors.Errorf("No helper pod is running on %v node", targetPod.Spec.NodeName)
}

// RemoveExtraSpaces remove all the extra spaces present in output of crictl commands
func RemoveExtraSpaces(arr []byte) []string {
	bytesSlice := make([]byte, len(arr))
	index := 0
	count := 0
	for i := 0; i < len(arr); i++ {
		count = 0
		for arr[i] == 32 {
			count++
			i++
			if i >= len(arr) {
				break
			}
		}
		if count > 1 {
			bytesSlice[index] = 32
			index++
		}
		bytesSlice[index] = arr[i]
		index++

	}
	return strings.Split(string(bytesSlice), "\n")
}
