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
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

//DeletePod deletes the specified pod and wait until it got terminated
func DeletePod(podName, podLabel, namespace string, timeout, delay int, clients clients.ClientSets) error {

	err := clients.KubeClient.CoreV1().Pods(namespace).Delete(podName, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	// waiting for the termination of the pod
	err = retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: podLabel})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Unable to delete the pod, err: %v", err)
			}
			return nil
		})

	return err
}

//DeleteAllPod deletes all the pods with matching labels and wait until all the pods got terminated
func DeleteAllPod(podLabel, namespace string, timeout, delay int, clients clients.ClientSets) error {

	err := clients.KubeClient.CoreV1().Pods(namespace).DeleteCollection(&v1.DeleteOptions{}, v1.ListOptions{LabelSelector: podLabel})

	if err != nil {
		return err
	}

	// waiting for the termination of the pod
	err = retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: podLabel})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Unable to delete the pod, err: %v", err)
			}
			return nil
		})

	return err
}

// GetChaosPodAnnotation will return the annotation on chaos pod
func GetChaosPodAnnotation(podName, namespace string, clients clients.ClientSets) (map[string]string, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(namespace).Get(podName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pod.Annotations, nil
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

	if err != nil && !k8serrors.IsNotFound(err) {
		return false, err
	} else if err != nil && k8serrors.IsNotFound(err) {
		return false, nil
	}
	return true, nil
}

// IsPodParentAnnotated check whether the target pod's parent is annotated or not
func IsPodParentAnnotated(clients clients.ClientSets, targetPod core_v1.Pod, chaosDetails *types.ChaosDetails) (bool, error) {

	switch chaosDetails.AppDetail.Kind {
	case "deployment", "deployments":
		deployList, err := clients.KubeClient.AppsV1().Deployments(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
		if err != nil || len(deployList.Items) == 0 {
			return false, errors.Errorf("No deployment found with matching label, err: %v", err)
		}
		for _, deploy := range deployList.Items {
			if deploy.ObjectMeta.Annotations[chaosDetails.AppDetail.AnnotationKey] == chaosDetails.AppDetail.AnnotationValue {
				rsOwnerRef := targetPod.OwnerReferences
				for _, own := range rsOwnerRef {
					if own.Kind == "ReplicaSet" {
						rs, err := clients.KubeClient.AppsV1().ReplicaSets(chaosDetails.AppDetail.Namespace).Get(own.Name, v1.GetOptions{})
						if err != nil {
							return false, err
						}
						ownerRef := rs.OwnerReferences
						for _, own := range ownerRef {
							if own.Kind == "Deployment" && own.Name == deploy.Name {
								return true, nil
							}
						}
					}
				}

			}
		}
		return false, nil
	case "statefulset", "statefulsets":
		stsList, err := clients.KubeClient.AppsV1().StatefulSets(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
		if err != nil || len(stsList.Items) == 0 {
			return false, errors.Errorf("No statefulset found with matching label, err: %v", err)
		}
		for _, sts := range stsList.Items {
			if sts.ObjectMeta.Annotations[chaosDetails.AppDetail.AnnotationKey] == chaosDetails.AppDetail.AnnotationValue {
				ownerRef := targetPod.OwnerReferences
				for _, own := range ownerRef {
					if own.Kind == "StatefulSet" && own.Name == sts.Name {
						return true, nil
					}
				}
			}
		}
	case "daemonset", "daemonsets":
		dsList, err := clients.KubeClient.AppsV1().DaemonSets(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
		if err != nil || len(dsList.Items) == 0 {
			return false, errors.Errorf("No daemonset found with matching label, err: %v", err)
		}
		for _, ds := range dsList.Items {
			if ds.ObjectMeta.Annotations[chaosDetails.AppDetail.AnnotationKey] == chaosDetails.AppDetail.AnnotationValue {
				ownerRef := targetPod.OwnerReferences
				for _, own := range ownerRef {
					if own.Kind == "DaemonSet" && own.Name == ds.Name {
						return true, nil
					}
				}
			}
		}
	case "deploymentconfig", "deploymentconfigs":
		gvrdc := schema.GroupVersionResource{
			Group:    "apps.openshift.io",
			Version:  "v1",
			Resource: "deploymentconfigs",
		}
		deploymentConfigList, err := clients.DynamicClient.Resource(gvrdc).Namespace(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
		if err != nil || len(deploymentConfigList.Items) == 0 {
			return false, errors.Errorf("No delploymentconfig found with matching labels, err: %v", err)
		}
		for _, dc := range deploymentConfigList.Items {
			annotations := dc.GetAnnotations()
			if annotations[chaosDetails.AppDetail.AnnotationKey] == chaosDetails.AppDetail.AnnotationValue {
				rcOwnerRef := targetPod.OwnerReferences
				for _, own := range rcOwnerRef {
					if own.Kind == "ReplicationController" {
						rc, err := clients.KubeClient.CoreV1().ReplicationControllers(chaosDetails.AppDetail.Namespace).Get(own.Name, v1.GetOptions{})
						if err != nil {
							return false, err
						}
						ownerRef := rc.OwnerReferences
						for _, own := range ownerRef {
							if own.Kind == "DeploymentConfig" && own.Name == dc.GetName() {
								return true, nil
							}
						}
					}
				}
			}
		}
	case "rollout", "rollouts":
		gvrro := schema.GroupVersionResource{
			Group:    "argoproj.io",
			Version:  "v1alpha1",
			Resource: "rollouts",
		}
		rolloutList, err := clients.DynamicClient.Resource(gvrro).Namespace(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
		if err != nil || len(rolloutList.Items) == 0 {
			return false, errors.Errorf("No rollouts found with matching labels, err: %v", err)
		}
		for _, ro := range rolloutList.Items {
			annotations := ro.GetAnnotations()
			if annotations[chaosDetails.AppDetail.AnnotationKey] == chaosDetails.AppDetail.AnnotationValue {
				rsOwnerRef := targetPod.OwnerReferences
				for _, own := range rsOwnerRef {
					if own.Kind == "ReplicaSet" {
						rs, err := clients.KubeClient.AppsV1().ReplicaSets(chaosDetails.AppDetail.Namespace).Get(own.Name, v1.GetOptions{})
						if err != nil {
							return false, err
						}
						ownerRef := rs.OwnerReferences
						for _, own := range ownerRef {
							if own.Kind == "Rollout" && own.Name == ro.GetName() {
								return true, nil
							}
						}
					}
				}

			}
		}
	default:
		return false, errors.Errorf("%v appkind is not supported", chaosDetails.AppDetail.Kind)
	}
	return false, nil
}

//FilterNonChaosPods remove the chaos pods(operator, runner) for the podList
// it filter when the applabels are not defined and it will select random pods from appns
func FilterNonChaosPods(podList core_v1.PodList, chaosDetails *types.ChaosDetails) core_v1.PodList {
	if chaosDetails.AppDetail.Label == "" {
		nonChaosPods := core_v1.PodList{}
		// ignore chaos pods
		for index, pod := range podList.Items {
			if !(pod.Labels["chaosUID"] == string(chaosDetails.ChaosUID) || pod.Labels["name"] == "chaos-operator") {
				nonChaosPods.Items = append(nonChaosPods.Items, podList.Items[index])
			}
		}
		return nonChaosPods
	}
	return podList
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
					isPodAnnotated, err := IsPodParentAnnotated(clients, pod, chaosDetails)
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
			isPodAnnotated, err := IsPodParentAnnotated(clients, pod, chaosDetails)
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
		err := DeletePod(podName, podLabel, chaosDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
		if err != nil {
			log.Errorf("Unable to delete the helper pod, err: %v", err)
		}
	}
}

// DeleteAllHelperPodBasedOnJobCleanupPolicy delete all the helper pods w/ matching label based on jobCleanupPolicy
func DeleteAllHelperPodBasedOnJobCleanupPolicy(podLabel string, chaosDetails *types.ChaosDetails, clients clients.ClientSets) {

	if chaosDetails.JobCleanupPolicy == "delete" {
		log.Info("[Cleanup]: Deleting all the helper pods")
		err := DeleteAllPod(podLabel, chaosDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
		if err != nil {
			log.Errorf("Unable to delete the helper pods, err: %v", err)
		}
	}
}
