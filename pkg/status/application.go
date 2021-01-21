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

	switch appLabel {
	case "":
		// Checking whether applications are healthy
		log.Info("[Status]: Checking whether applications are in healthy state")
		err := CheckPodAndContainerStatusInAppNs(appNs, timeout, delay, clients)
		if err != nil {
			return err
		}
	default:
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
	}
	return nil
}

// CheckPodAndContainerStatusInAppNs check status of pods and containers present in appns
func CheckPodAndContainerStatusInAppNs(appNs string, timeout, delay int, clients clients.ClientSets) error {
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{})
			if err != nil || len(podList.Items) == 0 {
				return errors.Errorf("Unable to find any pod in %v namespace, err: %v", appNs, err)
			}
			for _, pod := range podList.Items {
				if isChaosPod(pod.Labels) {
					continue
				}
				for _, container := range pod.Status.ContainerStatuses {
					if container.State.Terminated != nil {
						return errors.Errorf("container is in terminated state")
					}
					if container.Ready != true {
						return errors.Errorf("containers are not yet in running state")
					}
					log.InfoWithValues("[Status]: The Container status are as follows", logrus.Fields{
						"container": container.Name, "Pod": pod.Name, "Readiness": container.Ready})
				}
				if pod.Status.Phase != "Running" {
					return errors.Errorf("pods are not yet in running state")
				}
				log.InfoWithValues("[Status]: The Pod status are as follows", logrus.Fields{
					"Pod": pod.Name, "Phase": pod.Status.Phase})
			}
			return nil
		})
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

// CheckPodStatusPhase checks the status of the application pod
func CheckPodStatusPhase(appNs, appLabel string, timeout, delay int, clients clients.ClientSets, states ...string) error {
	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil || len(podList.Items) == 0 {
				return errors.Errorf("Unable to find the pods with matching labels, err: %v", err)
			}
			for _, pod := range podList.Items {
				isInState := false
				for _, state := range states {
					if string(pod.Status.Phase) == state {
						isInState = true
						break
					}
				}
				if !isInState {
					return errors.Errorf("Pod is not yet in %v state(s)", states)
				}
				log.InfoWithValues("[Status]: The status of Pods are as follows", logrus.Fields{
					"Pod": pod.Name, "Status": pod.Status.Phase})
			}
			return nil
		})
	if err != nil {
		return err
	}
	return nil
}

// CheckPodStatus checks the running status of the application pod
func CheckPodStatus(appNs, appLabel string, timeout, delay int, clients clients.ClientSets) error {
	return CheckPodStatusPhase(appNs, appLabel, timeout, delay, clients, "Running")
}

// CheckContainerStatus checks the status of the application container
func CheckContainerStatus(appNs, appLabel string, timeout, delay int, clients clients.ClientSets) error {

	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil || len(podList.Items) == 0 {
				return errors.Errorf("Unable to find the pods with matching labels, err: %v", err)
			}
			for _, pod := range podList.Items {
				for _, container := range pod.Status.ContainerStatuses {
					if container.State.Terminated != nil {
						return errors.Errorf("container is in terminated state")
					}
					if container.Ready != true {
						return errors.Errorf("containers are not yet in running state")
					}
					log.InfoWithValues("[Status]: The Container status are as follows", logrus.Fields{
						"container": container.Name, "Pod": pod.Name, "Readiness": container.Ready})
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
	failedPods := 0
	// It will wait till the completion of target container
	// it will retries until the target container completed or met the timeout(chaos duration)
	err := retry.
		Times(uint(duration)).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil || len(podList.Items) == 0 {
				return errors.Errorf("Unable to find the pods with matching labels, err: %v", err)
			}
			// it will check for the status of helper pod, if it is Succeeded and target container is completed then it will marked it as completed and return
			// if it is still running then it will check for the target container, as we can have multiple container inside helper pod (istio)
			// if the target container is in completed state(ready flag is false), then we will marked the helper pod as completed
			// we will retry till it met the timeout(chaos duration)
			failedPods = 0
			for _, pod := range podList.Items {
				podStatus = string(pod.Status.Phase)
				log.Infof("helper pod status: %v", podStatus)
				if podStatus != "Succeeded" && podStatus != "Failed" {
					for _, container := range pod.Status.ContainerStatuses {

						if container.Name == containerName && container.Ready {
							return errors.Errorf("Container is not completed yet")
						}
					}
				}
				if podStatus == "Pending" {
					return errors.Errorf("pod is in pending state")
				}
				log.InfoWithValues("[Status]: The running status of Pods are as follows", logrus.Fields{
					"Pod": pod.Name, "Status": podStatus})
				if podStatus == "Failed" {
					failedPods++
				}
			}
			return nil
		})
	if failedPods > 0 {
		return "Failed", err
	}
	return podStatus, err
}

// IsChaosPod check wheather the given pod is chaos pod or not
// based on labels present inside pod
func isChaosPod(labels map[string]string) bool {
	if labels["chaosUID"] != "" || labels["name"] == "chaos-operator" {
		return true
	}
	return false
}
