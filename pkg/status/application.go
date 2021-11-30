package status

import (
	"strings"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/annotation"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	logrus "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AUTStatusCheck checks the status of application under test
// if annotationCheck is true, it will check the status of the annotated pod only
// else it will check status of all pods with matching label
func AUTStatusCheck(appNs, appLabel, containerName string, timeout, delay int, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch chaosDetails.AppDetail.AnnotationCheck {
	case true:
		return AnnotatedApplicationsStatusCheck(appNs, appLabel, containerName, timeout, delay, clients, chaosDetails)
	default:
		switch appLabel {
		case "":
			// Checking whether applications are healthy
			log.Info("[Status]: No appLabels provided, skipping the application status checks")
		default:
			// Checking whether application containers are in ready state
			log.Info("[Status]: Checking whether application containers are in ready state")
			if err := CheckContainerStatus(appNs, appLabel, containerName, timeout, delay, clients); err != nil {
				return err
			}
			// Checking whether application pods are in running state
			log.Info("[Status]: Checking whether application pods are in running state")
			if err := CheckPodStatus(appNs, appLabel, timeout, delay, clients); err != nil {
				return err
			}
		}
	}
	return nil
}

// AnnotatedApplicationsStatusCheck checks the status of all the annotated applications with matching label
func AnnotatedApplicationsStatusCheck(appNs, appLabel, containerName string, timeout, delay int, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil {
				return errors.Errorf("Unable to find the pods with matching labels, err: %v", err)
			} else if len(podList.Items) == 0 {
				errors.Errorf("Unable to find the pods with matching labels")
			}
			for _, pod := range podList.Items {
				parentName, err := annotation.GetParentName(clients, pod, chaosDetails)
				if err != nil {
					return err
				}
				isParentAnnotated, err := annotation.IsParentAnnotated(clients, parentName, chaosDetails)
				if err != nil {
					return err
				}
				if isParentAnnotated {
					switch containerName {
					case "":
						for _, container := range pod.Status.ContainerStatuses {
							if container.State.Terminated != nil {
								return errors.Errorf("container is in terminated state")
							}
							if !container.Ready {
								return errors.Errorf("containers are not yet in running state")
							}
							log.InfoWithValues("[Status]: The Container status are as follows", logrus.Fields{
								"container": container.Name, "Pod": pod.Name, "Readiness": container.Ready})
						}
					default:
						for _, container := range pod.Status.ContainerStatuses {
							if containerName == container.Name {
								if container.State.Terminated != nil {
									return errors.Errorf("container is in terminated state")
								}
								if !container.Ready {
									return errors.Errorf("containers are not yet in running state")
								}
								log.InfoWithValues("[Status]: The Container status are as follows", logrus.Fields{
									"container": container.Name, "Pod": pod.Name, "Readiness": container.Ready})
							}
						}
					}
					if pod.Status.Phase != "Running" {
						return errors.Errorf("%v pod is not yet in running state", pod.Name)
					}
					log.InfoWithValues("[Status]: The status of Pods are as follows", logrus.Fields{
						"Pod": pod.Name, "Status": pod.Status.Phase})
				}
			}
			return nil
		})
}

// CheckApplicationStatus checks the status of the AUT
func CheckApplicationStatus(appNs, appLabel string, timeout, delay int, clients clients.ClientSets) error {

	switch appLabel {
	case "":
		// Checking whether applications are healthy
		log.Info("[Status]: No appLabels provided, skipping the application status checks")
	default:
		// Checking whether application containers are in ready state
		log.Info("[Status]: Checking whether application containers are in ready state")
		if err := CheckContainerStatus(appNs, appLabel, "", timeout, delay, clients); err != nil {
			return err
		}
		// Checking whether application pods are in running state
		log.Info("[Status]: Checking whether application pods are in running state")
		if err := CheckPodStatus(appNs, appLabel, timeout, delay, clients); err != nil {
			return err
		}
	}
	return nil
}

// CheckAuxiliaryApplicationStatus checks the status of the Auxiliary applications
func CheckAuxiliaryApplicationStatus(AuxiliaryAppDetails string, timeout, delay int, clients clients.ClientSets) error {

	AuxiliaryAppInfo := strings.Split(AuxiliaryAppDetails, ",")

	for _, val := range AuxiliaryAppInfo {
		AppInfo := strings.Split(val, ":")
		if err := CheckApplicationStatus(AppInfo[0], AppInfo[1], timeout, delay, clients); err != nil {
			return err
		}
	}
	return nil
}

// CheckPodStatusPhase checks the status of the application pod
func CheckPodStatusPhase(appNs, appLabel string, timeout, delay int, clients clients.ClientSets, states ...string) error {
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil {
				return errors.Errorf("Unable to find the pods with matching labels, err: %v", err)
			} else if len(podList.Items) == 0 {
				errors.Errorf("Unable to find the pods with matching labels")
			}

			for _, pod := range podList.Items {
				isInState := isOneOfState(string(pod.Status.Phase), states)
				if !isInState {
					return errors.Errorf("Pod is not yet in %v state(s)", states)
				}
				log.InfoWithValues("[Status]: The status of Pods are as follows", logrus.Fields{
					"Pod": pod.Name, "Status": pod.Status.Phase})
			}
			return nil
		})
}

// isOneOfState check for the string should be present inside given list
func isOneOfState(podState string, states []string) bool {
	for i := range states {
		if podState == states[i] {
			return true
		}
	}
	return false
}

// CheckPodStatus checks the running status of the application pod
func CheckPodStatus(appNs, appLabel string, timeout, delay int, clients clients.ClientSets) error {
	return CheckPodStatusPhase(appNs, appLabel, timeout, delay, clients, "Running")
}

// CheckContainerStatus checks the status of the application container
func CheckContainerStatus(appNs, appLabel, containerName string, timeout, delay int, clients clients.ClientSets) error {

	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil {
				return errors.Errorf("Unable to find the pods with matching labels, err: %v", err)
			} else if len(podList.Items) == 0 {
				errors.Errorf("Unable to find the pods with matching labels")
			}
			for _, pod := range podList.Items {
				switch containerName {
				case "":
					if err := validateAllContainerStatus(pod.Name, pod.Status.ContainerStatuses); err != nil {
						return err
					}
				default:
					if err := validateContainerStatus(containerName, pod.Name, pod.Status.ContainerStatuses); err != nil {
						return err
					}
				}
			}
			return nil
		})
}

// validateContainerStatus verify that the provided container should be in ready state
func validateContainerStatus(containerName, podName string, ContainerStatuses []v1.ContainerStatus) error {
	for _, container := range ContainerStatuses {
		if container.Name == containerName {
			if container.State.Terminated != nil {
				return errors.Errorf("container is in terminated state")
			}
			if !container.Ready {
				return errors.Errorf("containers are not yet in running state")
			}
			log.InfoWithValues("[Status]: The Container status are as follows", logrus.Fields{
				"container": container.Name, "Pod": podName, "Readiness": container.Ready})
		}
	}
	return nil
}

// validateAllContainerStatus verify that the all the containers should be in ready state
func validateAllContainerStatus(podName string, ContainerStatuses []v1.ContainerStatus) error {
	for _, container := range ContainerStatuses {
		if err := validateContainerStatus(container.Name, podName, ContainerStatuses); err != nil {
			return err
		}
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
			if err != nil {
				return errors.Errorf("Unable to find the pods with matching labels, err: %v", err)
			} else if len(podList.Items) == 0 {
				errors.Errorf("Unable to find the pods with matching labels")
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

// CheckHelperStatus checks the status of the helper pod
// and wait until the helper pod comes to one of the {running,completed,failed} states
func CheckHelperStatus(appNs, appLabel string, timeout, delay int, clients clients.ClientSets) error {

	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil {
				return errors.Errorf("unable to find the pods with matching labels, err: %v", err)
			} else if len(podList.Items) == 0 {
				errors.Errorf("Unable to find the pods with matching labels")
			}
			for _, pod := range podList.Items {
				podStatus := string(pod.Status.Phase)
				switch strings.ToLower(podStatus) {
				case "running", "succeeded", "failed":
					log.Infof("%v helper pod is in %v state", pod.Name, podStatus)
				default:
					return errors.Errorf("%v pod is in %v state", pod.Name, podStatus)
				}
				for _, container := range pod.Status.ContainerStatuses {
					if container.State.Terminated != nil && container.State.Terminated.Reason != "Completed" && container.State.Terminated.Reason != "Error" {
						return errors.Errorf("container is terminated with %v reason", container.State.Terminated.Reason)
					}
				}
			}
			return nil
		})
}
