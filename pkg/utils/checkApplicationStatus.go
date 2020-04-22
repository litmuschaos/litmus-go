package utils

import (
	"k8s.io/klog"
	"strings"
	"time"

	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckApplicationStatus checks the status of the AUT
func CheckApplicationStatus(appNs string, appLabel string, clients ClientSets) error {

	// Checking whether application pods are in running state
	klog.V(0).Infof("[Status]: Checking whether application pods are in running state")
	err := CheckPodStatus(appNs, appLabel, clients)
	if err != nil {
		return err
	}
	// Checking whether application containers are in running state
	klog.V(0).Infof("[Status]: Checking whether application containers are in running state")
	err = CheckContainerStatus(appNs, appLabel, clients)
	if err != nil {
		return err
	}
	return nil
}

// CheckAuxiliaryApplicationStatus checks the status of the Auxiliary applications
func CheckAuxiliaryApplicationStatus(experimentsDetails *ExperimentDetails, clients ClientSets) error {

	AuxiliaryAppInfo := strings.Split(experimentsDetails.AuxiliaryAppInfo, ",")

	for _, val := range AuxiliaryAppInfo {
		AppInfo := strings.Split(val, ":")
		err := CheckApplicationStatus(AppInfo[0], AppInfo[1], clients)
		if err != nil {
			return err
		}

	}
	return nil
}

// CheckPodStatus checks the status of the application pod
func CheckPodStatus(appNs string, appLabel string, clients ClientSets) error {
	err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil {
				return err
			}
			err = nil
			for _, pod := range podSpec.Items {
				if string(pod.Status.Phase) != "Running" {
					return errors.Errorf("Pod is not yet in running state")
				}
				klog.V(0).Infof(" %v Pod is in %v State", pod.Name, pod.Status.Phase)
			}
			return nil
		})
	if err != nil {
		return err
	}
	return nil
}

// CheckContainerStatus checks the status of the application container
func CheckContainerStatus(appNs string, appLabel string, clients ClientSets) error {
	err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{LabelSelector: appLabel})
			if err != nil {
				return err
			}
			err = nil
			for _, pod := range podSpec.Items {

				for _, container := range pod.Status.ContainerStatuses {
					if container.Ready != true {
						return errors.Errorf("containers are not yet in running state")
					}
					klog.V(0).Infof(" %v container of pod %v is in %v State", container.Name, pod.Name, pod.Status.Phase)
				}
			}
			return nil
		})
	if err != nil {
		return err
	}
	return nil
}
