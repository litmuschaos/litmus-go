package status

import (
	"strings"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	logrus "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckApplicationStatus checks the status of the AUT
func CheckApplicationStatus(appNs, appLabel string, timeout, delay int, clients clients.ClientSets) error {

	// Checking whether application containers are in ready state
	log.Info("[Status]: Checking whether application containers are in ready state")
	err := CheckContainerStatus(appNs, appLabel, timeout, delay, clients)
	if err != nil {
		return err
	}
	// Checking whether application pods are in running state
	log.Info("[Status]: Checking whether application pods are in running state")
	err = CheckPodStatus(appNs, appLabel, timeout, delay, clients)
	if err != nil {
		return err
	}

	return nil
}

// CheckAuxiliaryApplicationStatus checks the status of the Auxiliary applications
func CheckAuxiliaryApplicationStatus(AuxiliaryAppDetails string, timeout, delay int, clients clients.ClientSets) error {

	AuxiliaryAppInfo := strings.Split(AuxiliaryAppDetails, ",")

	for _, val := range AuxiliaryAppInfo {
		AppInfo := strings.Split(val, ":")
		err := CheckApplicationStatus(AppInfo[0], AppInfo[1], timeout, delay, clients)
		if err != nil {
			return err
		}

	}
	return nil
}

// CheckPodStatus checks the running status of the application pod
func CheckPodStatus(appNs, appLabel string, timeout, delay int, clients clients.ClientSets) error {
	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil || len(podSpec.Items) == 0 {
				return errors.Errorf("Unable to find the pods with matching labels, err: %v", err)
			}
			for _, pod := range podSpec.Items {
				if string(pod.Status.Phase) != "Running" {
					return errors.Errorf("Pod is not yet in running state")
				}
				log.InfoWithValues("[Status]: The running status of Pods are as follows", logrus.Fields{
					"Pod": pod.Name, "Status": pod.Status.Phase})
			}
			return nil
		})
	if err != nil {
		return err
	}
	return nil
}

// CheckContainerStatus checks the status of the application container
func CheckContainerStatus(appNs, appLabel string, timeout, delay int, clients clients.ClientSets) error {

	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil || len(podSpec.Items) == 0 {
				return errors.Errorf("Unable to find the pods with matching labels, err: %v", err)
			}
			for _, pod := range podSpec.Items {
				for _, container := range pod.Status.ContainerStatuses {
					if container.State.Terminated != nil {
						return errors.Errorf("container is in terminated state")
					}
					if container.Ready != true {
						return errors.Errorf("containers are not yet in running state")
					}
					log.InfoWithValues("[Status]: The Container status are as follows", logrus.Fields{
						"container": container.Name, "Pod": pod.Name, "Ready": container.Ready})
				}
			}
			return nil
		})
	if err != nil {
		return err
	}
	return nil
}

// WaitForCompletion wait until the completion of pod
func WaitForCompletion(appNs, appLabel string, clients clients.ClientSets, duration int, containerName string) (string, error) {
	var podStatus string
	// It will wait till the completion of target container
	// it will retries until the target container completed or met the timeout(chaos duration)
	err := retry.
		Times(uint(duration)).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil || len(podSpec.Items) == 0 {
				return errors.Errorf("Unable to find the pods with matching labels, err: %v", err)
			}
			// it will check for the status of helper pod, if it is Succeeded and target container is completed then it will marked it as completed and return
			// if it is still running then it will check for the target container, as we can have multiple container inside helper pod (istio)
			// if the target container is in completed state(ready flag is false), then we will marked the helper pod as completed
			// we will retry till it met the timeout(chaos duration)
			for _, pod := range podSpec.Items {
				podStatus = string(pod.Status.Phase)
				log.Infof("helper pod status: %v", podStatus)
				if podStatus != "Succeeded" && podStatus != "Failed" {
					for _, container := range pod.Status.ContainerStatuses {

						if container.Name == containerName && container.Ready {
							return errors.Errorf("Container is not completed yet")
						}
					}
				}
				log.InfoWithValues("[Status]: The running status of Pods are as follows", logrus.Fields{
					"Pod": pod.Name, "Status": podStatus})
			}
			return nil
		})
	if err != nil {
		return "", err
	}
	return podStatus, nil
}
