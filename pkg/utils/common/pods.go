package common

import (
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/annotation"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	core_v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//DeletePod deletes the specified pod and wait until it got terminated
func DeletePod(podName, podLabel, namespace string, timeout, delay int, clients clients.ClientSets) error {

	if err := clients.KubeClient.CoreV1().Pods(namespace).Delete(podName, &v1.DeleteOptions{}); err != nil {
		return err
	}

	// waiting for the termination of the pod
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: podLabel})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Unable to delete the pod, err: %v", err)
			}
			return nil
		})
}

//DeleteAllPod deletes all the pods with matching labels and wait until all the pods got terminated
func DeleteAllPod(podLabel, namespace string, timeout, delay int, clients clients.ClientSets) error {

	if err := clients.KubeClient.CoreV1().Pods(namespace).DeleteCollection(&v1.DeleteOptions{}, v1.ListOptions{LabelSelector: podLabel}); err != nil {
		return err
	}

	// waiting for the termination of the pod
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: podLabel})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Unable to delete the pod, err: %v", err)
			}
			return nil
		})
}

// GetChaosPodAnnotation will return the annotation on chaos pod
func GetChaosPodAnnotation(podName, namespace string, clients clients.ClientSets) (map[string]string, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(namespace).Get(podName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pod.Annotations, nil
}

// GetImagePullSecrets return the imagePullSecrets from the experiment pod
func GetImagePullSecrets(podName, namespace string, clients clients.ClientSets) ([]core_v1.LocalObjectReference, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(namespace).Get(podName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pod.Spec.ImagePullSecrets, nil
}

// GetChaosPodResourceRequirements will return the resource requirements on chaos pod
func GetChaosPodResourceRequirements(podName, containerName, namespace string, clients clients.ClientSets) (core_v1.ResourceRequirements, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(namespace).Get(podName, v1.GetOptions{})
	if err != nil {
		return core_v1.ResourceRequirements{}, err
	}
	for _, container := range pod.Spec.Containers {
		// The name of chaos container is always same as job name
		// <experiment-name>-<runid>
		if strings.Contains(container.Name, containerName) {
			return container.Resources, nil
		}
	}
	return core_v1.ResourceRequirements{}, errors.Errorf("No container found with %v name in target pod", containerName)
}

// VerifyExistanceOfPods check the availibility of list of pods
func VerifyExistanceOfPods(namespace, pods string, clients clients.ClientSets) (bool, error) {

	if pods == "" {
		return false, nil
	}

	podList := strings.Split(pods, ",")
	for index := range podList {
		isPodsAvailable, err := CheckForAvailibiltyOfPod(namespace, podList[index], clients)
		if err != nil {
			return false, err
		}
		if !isPodsAvailable {
			return isPodsAvailable, nil
		}
	}
	return true, nil
}

//GetPodList check for the availibilty of the target pod for the chaos execution
// if the target pod is not defined it will derive the random target pod list using pod affected percentage
func GetPodList(targetPods string, podAffPerc int, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {
	realpods := core_v1.PodList{}
	podList, err := clients.KubeClient.CoreV1().Pods(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
	if err != nil || len(podList.Items) == 0 {
		return core_v1.PodList{}, errors.Wrapf(err, "Failed to find the pod with matching labels in %v namespace", chaosDetails.AppDetail.Namespace)
	}

	isPodsAvailable, err := VerifyExistanceOfPods(chaosDetails.AppDetail.Namespace, targetPods, clients)
	if err != nil {
		return core_v1.PodList{}, err
	}

	// getting the pod, if the target pods is defined
	// else select a random target pod from the specified labels
	switch isPodsAvailable {
	case true:
		podList, err := GetTargetPodsWhenTargetPodsENVSet(targetPods, clients, chaosDetails)
		if err != nil {
			return core_v1.PodList{}, err
		}
		realpods.Items = append(realpods.Items, podList.Items...)
	default:
		nonChaosPods := FilterNonChaosPods(*podList, chaosDetails)
		realpods, err = GetTargetPodsWhenTargetPodsENVNotSet(podAffPerc, clients, nonChaosPods, chaosDetails)
		if err != nil {
			return core_v1.PodList{}, err
		}
	}
	log.Infof("[Chaos]:Number of pods targeted: %v", strconv.Itoa(len(realpods.Items)))

	return realpods, nil
}

// CheckForAvailibiltyOfPod check the availibility of the specified pod
func CheckForAvailibiltyOfPod(namespace, name string, clients clients.ClientSets) (bool, error) {

	if name == "" {
		return false, nil
	}
	_, err := clients.KubeClient.CoreV1().Pods(namespace).Get(name, v1.GetOptions{})

	if err != nil && k8serrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

//FilterNonChaosPods remove the chaos pods(operator, runner) for the podList
// it filter when the applabels are not defined and it will select random pods from appns
func FilterNonChaosPods(podList core_v1.PodList, chaosDetails *types.ChaosDetails) core_v1.PodList {
	nonChaosPods := core_v1.PodList{}
	// ignore chaos pods
	for index, pod := range podList.Items {
		if pod.Labels["chaosUID"] == "" && pod.Labels["name"] != "chaos-operator" {
			nonChaosPods.Items = append(nonChaosPods.Items, podList.Items[index])
		}
	}
	return nonChaosPods
}

// GetTargetPodsWhenTargetPodsENVSet derive the specific target pods, if TARGET_PODS env is set
func GetTargetPodsWhenTargetPodsENVSet(targetPods string, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
	if err != nil || len(podList.Items) == 0 {
		return core_v1.PodList{}, errors.Wrapf(err, "Failed to find the pods with matching labels in %v namespace", chaosDetails.AppDetail.Namespace)
	}

	targetPodsList := strings.Split(targetPods, ",")
	realPods := core_v1.PodList{}

	for _, pod := range podList.Items {
		for index := range targetPodsList {
			if targetPodsList[index] == pod.Name {
				switch chaosDetails.AppDetail.AnnotationCheck {
				case true:
					isPodAnnotated, err := annotation.IsPodParentAnnotated(clients, pod, chaosDetails)
					if err != nil {
						return core_v1.PodList{}, err
					}
					if !isPodAnnotated {
						return core_v1.PodList{}, errors.Errorf("%v target pods are not annotated", targetPods)
					}
				}
				realPods.Items = append(realPods.Items, pod)
			}
		}
	}
	return realPods, nil
}

// GetTargetPodsWhenTargetPodsENVNotSet derives the random target pod list, if TARGET_PODS env is not set
func GetTargetPodsWhenTargetPodsENVNotSet(podAffPerc int, clients clients.ClientSets, nonChaosPods core_v1.PodList, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {
	filteredPods := core_v1.PodList{}
	realPods := core_v1.PodList{}

	switch chaosDetails.AppDetail.AnnotationCheck {
	case true:
		for _, pod := range nonChaosPods.Items {
			isPodAnnotated, err := annotation.IsPodParentAnnotated(clients, pod, chaosDetails)
			if err != nil {
				return core_v1.PodList{}, err
			}
			if isPodAnnotated {
				filteredPods.Items = append(filteredPods.Items, pod)
			}
		}
		if len(filteredPods.Items) == 0 {
			return filteredPods, errors.Errorf("No annotated target pod found")
		}
	default:
		filteredPods = nonChaosPods
	}

	newPodListLength := math.Maximum(1, math.Adjustment(podAffPerc, len(filteredPods.Items)))
	rand.Seed(time.Now().UnixNano())

	// it will generate the random podlist
	// it starts from the random index and choose requirement no of pods next to that index in a circular way.
	index := rand.Intn(len(filteredPods.Items))
	for i := 0; i < newPodListLength; i++ {
		realPods.Items = append(realPods.Items, filteredPods.Items[index])
		index = (index + 1) % len(filteredPods.Items)
	}
	return realPods, nil
}

// DeleteHelperPodBasedOnJobCleanupPolicy deletes specific helper pod based on jobCleanupPolicy
func DeleteHelperPodBasedOnJobCleanupPolicy(podName, podLabel string, chaosDetails *types.ChaosDetails, clients clients.ClientSets) {

	if chaosDetails.JobCleanupPolicy == "delete" {
		log.Infof("[Cleanup]: Deleting %v helper pod", podName)
		if err := DeletePod(podName, podLabel, chaosDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			log.Errorf("Unable to delete the helper pod, err: %v", err)
		}
	}
}

// DeleteAllHelperPodBasedOnJobCleanupPolicy delete all the helper pods w/ matching label based on jobCleanupPolicy
func DeleteAllHelperPodBasedOnJobCleanupPolicy(podLabel string, chaosDetails *types.ChaosDetails, clients clients.ClientSets) {

	if chaosDetails.JobCleanupPolicy == "delete" {
		log.Info("[Cleanup]: Deleting all the helper pods")
		if err := DeleteAllPod(podLabel, chaosDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			log.Errorf("Unable to delete the helper pods, err: %v", err)
		}
	}
}

// GetServiceAccount derive the serviceAccountName for the helper pod
func GetServiceAccount(chaosNamespace, chaosPodName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(chaosNamespace).Get(chaosPodName, v1.GetOptions{})
	if err != nil {
		return "", err
	}
	return pod.Spec.ServiceAccountName, nil
}

//GetTargetContainer will fetch the container name from application pod
//This container will be used as target container
func GetTargetContainer(appNamespace, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(appNamespace).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", err
	}
	return pod.Spec.Containers[0].Name, nil
}

//GetContainerID  derive the container id of the application container
func GetContainerID(appNamespace, targetPod, targetContainer string, clients clients.ClientSets) (string, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(appNamespace).Get(targetPod, v1.GetOptions{})
	if err != nil {
		return "", err
	}

	var containerID string

	// filtering out the container id from the details of containers inside containerStatuses of the given pod
	// container id is present in the form of <runtime>://<container-id>
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == targetContainer {
			containerID = strings.Split(container.ContainerID, "//")[1]
			break
		}
	}

	log.Infof("container ID of %v container, containerID: %v", targetContainer, containerID)
	return containerID, nil
}

// CheckContainerStatus checks the status of the application container
func CheckContainerStatus(appNamespace, appName string, clients clients.ClientSets) error {
	return retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(appNamespace).Get(appName, v1.GetOptions{})
			if err != nil {
				return errors.Errorf("unable to find the pod with name %v, err: %v", appName, err)
			}
			for _, container := range pod.Status.ContainerStatuses {
				if !container.Ready {
					return errors.Errorf("containers are not yet in running state")
				}
				log.InfoWithValues("The running status of container are as follows", logrus.Fields{
					"container": container.Name, "Pod": pod.Name, "Status": pod.Status.Phase})
			}
			return nil
		})
}
