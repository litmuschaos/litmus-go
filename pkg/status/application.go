package status

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/litmuschaos/litmus-go/pkg/workloads"
	logrus "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AUTStatusCheck checks the status of application under test
// if annotationCheck is true, it will check the status of the annotated pod only
// else it will check status of all pods with matching label
func AUTStatusCheck(clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	if chaosDetails.AppDetail == nil || (len(chaosDetails.AppDetail) == 1 && chaosDetails.AppDetail[0].Kind == "KIND") {
		log.Info("[Status]: No appLabels provided, skipping the application status checks")
		return nil
	}

	for _, target := range chaosDetails.AppDetail {
		switch target.Kind {
		case "pod":
			for _, name := range target.Names {
				if err := CheckApplicationStatusesByPodName(target.Namespace, name, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
					return stacktrace.Propagate(err, "could not check application status by pod names")
				}
			}
		default:
			if target.Labels != nil {
				for _, label := range target.Labels {
					if err := CheckApplicationStatusesByLabels(target.Namespace, label, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
						return stacktrace.Propagate(err, "could not check application status by labels")
					}
				}
			} else {
				if err := CheckApplicationStatusesByWorkloadName(target, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
					return stacktrace.Propagate(err, "could not check application status by workload names")
				}
			}
		}
	}

	return nil
}

// CheckApplicationStatusesByLabels checks the status of the AUT
func CheckApplicationStatusesByLabels(appNs, appLabel string, timeout, delay int, clients clients.ClientSets) error {

	switch appLabel {
	case "":
		// Checking whether applications are healthy
		log.Info("[Status]: No appLabels provided, skipping the application status checks")
	default:
		// Checking whether application containers are in ready state
		log.Info("[Status]: Checking whether application containers are in ready state")
		if err := CheckContainerStatus(appNs, appLabel, "", timeout, delay, clients); err != nil {
			return stacktrace.Propagate(err, "could not check container status")
		}
		// Checking whether application pods are in running state
		log.Info("[Status]: Checking whether application pods are in running state")
		if err := CheckPodStatus(appNs, appLabel, timeout, delay, clients); err != nil {
			return stacktrace.Propagate(err, "could not check pod status")
		}
	}
	return nil
}

// CheckAuxiliaryApplicationStatus checks the status of the Auxiliary applications
func CheckAuxiliaryApplicationStatus(AuxiliaryAppDetails string, timeout, delay int, clients clients.ClientSets) error {

	AuxiliaryAppInfo := strings.Split(AuxiliaryAppDetails, ",")

	for _, val := range AuxiliaryAppInfo {
		AppInfo := strings.Split(val, ":")
		if err := CheckApplicationStatusesByLabels(AppInfo[0], AppInfo[1], timeout, delay, clients); err != nil {
			return stacktrace.Propagate(err, "could not check auxiliary application status")
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
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(context.Background(), metav1.ListOptions{LabelSelector: appLabel})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podLabels: %s, namespace: %s}", appLabel, appNs), Reason: err.Error()}
			} else if len(podList.Items) == 0 {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podLabels: %s, namespace: %s}", appLabel, appNs), Reason: "no pod found with matching labels"}
			}

			for _, pod := range podList.Items {
				isInState := isOneOfState(string(pod.Status.Phase), states)
				if !isInState {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, appNs), Reason: fmt.Sprintf("pod is not in [%v] states", states)}
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
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(context.Background(), metav1.ListOptions{LabelSelector: appLabel})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podLabels: %s, namespace: %v}", appLabel, appNs), Reason: err.Error()}
			} else if len(podList.Items) == 0 {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podLabels: %s, namespace: %v}", appLabel, appNs), Reason: "no pod found with matching labels"}
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
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("podName: %s, containerName: %s", podName, containerName), Reason: "container is in terminated state"}
			}
			if !container.Ready {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("podName: %s, containerName: %s", podName, containerName), Reason: "container is not in running state"}
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
func WaitForCompletion(appNs, appLabel string, clients clients.ClientSets, duration int, containerNames ...string) (string, error) {
	var podStatus string
	failedPods := 0
	// It will wait till the completion of target container
	// it will retry until the target container completed or met the timeout(chaos duration)
	err := retry.
		Times(uint(duration)).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(context.Background(), metav1.ListOptions{LabelSelector: appLabel})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podLabel: %s, namespace: %s}", appLabel, appNs), Reason: err.Error()}
			} else if len(podList.Items) == 0 {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podLabel: %s, namespace: %s}", appLabel, appNs), Reason: "no pod with matching label"}
			}
			// it will check for the status of helper pod, if it is Succeeded and target container is completed then it will mark it as completed and return
			// if it is still running then it will check for the target container, as we can have multiple container inside helper pod (istio)
			// if the target container is in completed state(ready flag is false), then we will mark the helper pod as completed
			// we will retry till it met the timeout(chaos duration)
			failedPods = 0
			for _, pod := range podList.Items {
				podStatus = string(pod.Status.Phase)
				log.Infof("helper pod status: %v", podStatus)
				if podStatus != "Succeeded" && podStatus != "Failed" {
					for _, container := range pod.Status.ContainerStatuses {
						if Contains(container.Name, containerNames) {
							if container.Ready {
								return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", pod.Name, pod.Namespace, container.Name), Reason: "container is not completed within timeout"}
							} else if container.State.Terminated != nil && container.State.Terminated.ExitCode == 1 {
								podStatus = "Failed"
								break
							}
						}
					}
				}
				if podStatus == "Pending" {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: "pod is in pending state"}
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
			podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(context.Background(), metav1.ListOptions{LabelSelector: appLabel})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podLabel: %s, namespace: %s}", appLabel, appNs), Reason: fmt.Sprintf("helper status check failed: %s", err.Error())}
			} else if len(podList.Items) == 0 {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podLabel: %s, namespace: %s}", appLabel, appNs), Reason: "helper status check failed: no pods found with mathcing labels"}
			}
			for _, pod := range podList.Items {
				podStatus := string(pod.Status.Phase)
				switch strings.ToLower(podStatus) {
				case "running", "succeeded", "failed":
					log.Infof("%v helper pod is in %v state", pod.Name, podStatus)
				default:
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("helper pod is in %s state", podStatus)}
				}
				for _, container := range pod.Status.ContainerStatuses {
					if container.State.Terminated != nil && container.State.Terminated.Reason != "Completed" && container.State.Terminated.Reason != "Error" {
						return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("helper pod's container is in terminated state with %s reason", container.State.Terminated.Reason)}
					}
				}
			}
			return nil
		})
}

func CheckPodStatusByPodName(appNs, appName string, timeout, delay int, clients clients.ClientSets) error {
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(appNs).Get(context.Background(), appName, metav1.GetOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("podName: %v, namespace: %v", appName, appNs), Reason: err.Error()}
			}

			if pod.Status.Phase != v1.PodRunning {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("podName: %v, namespace: %v", appName, appNs), Reason: "pod is not in Running state"}
			}
			log.InfoWithValues("[Status]: The status of Pods are as follows", logrus.Fields{
				"Pod": pod.Name, "Status": pod.Status.Phase})

			return nil
		})
}

func CheckAllContainerStatusesByPodName(appNs, appName string, timeout, delay int, clients clients.ClientSets) error {
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(appNs).Get(context.Background(), appName, metav1.GetOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("podName: %v, namespace: %v", appName, appNs), Reason: err.Error()}
			}
			if err := validateAllContainerStatus(pod.Name, pod.Status.ContainerStatuses); err != nil {
				return err
			}
			return nil
		})
}

func CheckApplicationStatusesByWorkloadName(target types.AppDetails, timeout, delay int, clients clients.ClientSets) error {

	pods, err := workloads.GetPodsFromWorkloads(target, clients)
	if err != nil {
		return stacktrace.Propagate(err, "could not get pods from workloads")
	}
	for _, pod := range pods.Items {
		if err := CheckApplicationStatusesByPodName(target.Namespace, pod.Name, timeout, delay, clients); err != nil {
			return stacktrace.Propagate(err, "could not check application status by pod name")
		}
	}
	return nil
}

func CheckUnTerminatedPodStatusesByWorkloadName(target types.AppDetails, timeout, delay int, clients clients.ClientSets) error {

	pods, err := workloads.GetPodsFromWorkloads(target, clients)
	if err != nil {
		return stacktrace.Propagate(err, "could not get pods by workload names")
	}
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp == nil {
			if err := CheckApplicationStatusesByPodName(target.Namespace, pod.Name, timeout, delay, clients); err != nil {
				return stacktrace.Propagate(err, "could not check application status by pod name")
			}
		}
	}
	return nil
}

func CheckApplicationStatusesByPodName(appNs, pod string, timeout, delay int, clients clients.ClientSets) error {
	// Checking whether application containers are in ready state
	log.Info("[Status]: Checking whether application containers are in ready state")
	if err := CheckAllContainerStatusesByPodName(appNs, pod, timeout, delay, clients); err != nil {
		return stacktrace.Propagate(err, "could not check container statuses by pod name")
	}
	// Checking whether application pods are in running state
	log.Info("[Status]: Checking whether application pods are in running state")
	if err := CheckPodStatusByPodName(appNs, pod, timeout, delay, clients); err != nil {
		return stacktrace.Propagate(err, "could not check pod status by pod name")
	}
	return nil
}

// Contains checks whether slice contains the expected value
func Contains(val interface{}, slice interface{}) bool {
	if slice == nil {
		return false
	}
	for i := 0; i < reflect.ValueOf(slice).Len(); i++ {
		// it matches the expected value with the ith index value of slice
		if fmt.Sprintf("%v", reflect.ValueOf(val).Interface()) == fmt.Sprintf("%v", reflect.ValueOf(slice).Index(i).Interface()) {
			return true
		}
	}
	return false
}
