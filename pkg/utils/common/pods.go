package common

import (
	"math/rand"
	"strconv"
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

//GetPodList check for the availibilty of the target pod for the chaos execution
// if the target pod is not defined it will derive the random target pod list using pod affected percentage
func GetPodList(targetPod string, podAffPerc int, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {
	realpods := core_v1.PodList{}
	podList, err := clients.KubeClient.CoreV1().Pods(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
	if err != nil || len(podList.Items) == 0 {
		return core_v1.PodList{}, errors.Wrapf(err, "Failed to find the pod with matching labels in %v namespace", chaosDetails.AppDetail.Namespace)
	}
	isPodAvailable, err := CheckForAvailibiltyOfPod(chaosDetails.AppDetail.Namespace, targetPod, clients)
	if err != nil {
		return core_v1.PodList{}, err
	}

	// getting the pod, if the target pod is defined
	// else select a random target pod from the specified labels
	switch isPodAvailable {
	case true:
		pod, err := GetTargetPodsWhenTargetPodsENVSet(targetPod, clients, chaosDetails)
		if err != nil {
			return core_v1.PodList{}, err
		}
		realpods.Items = append(realpods.Items, pod)
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

// IsPodAnnotated ...
func IsPodAnnotated(clients clients.ClientSets, targetPod core_v1.Pod, chaosDetails *types.ChaosDetails) (bool, error) {

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
				ownerRef := dc.GetOwnerReferences()
				for _, own := range ownerRef {
					if own.Kind == "DeploymentConfig" && own.Name == dc.GetName() {
						return true, nil
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
				ownerRef := ro.GetOwnerReferences()
				for _, own := range ownerRef {
					if own.Kind == "Rollout" && own.Name == ro.GetName() {
						return true, nil
					}
				}
			}
		}
	default:
		return false, errors.Errorf("%v appkind is not supported", chaosDetails.AppDetail.Kind)
	}
	return false, nil
}

//FilterNonChaosPods ...
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

// GetTargetPodsWhenTargetPodsENVSet ...
func GetTargetPodsWhenTargetPodsENVSet(targetPod string, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (core_v1.Pod, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(chaosDetails.AppDetail.Namespace).Get(targetPod, v1.GetOptions{})
	if err != nil {
		return core_v1.Pod{}, err
	}

	switch chaosDetails.AppDetail.AnnotationCheck {
	case true:
		isPodAnnotated, err := IsPodAnnotated(clients, *pod, chaosDetails)
		if err != nil {
			return core_v1.Pod{}, err
		}
		if !isPodAnnotated {
			return core_v1.Pod{}, errors.Errorf("%v target pod is not annotated", targetPod)
		}
	default:
		return *pod, nil
	}

	return *pod, nil
}

// GetTargetPodsWhenTargetPodsENVNotSet ...
func GetTargetPodsWhenTargetPodsENVNotSet(podAffPerc int, clients clients.ClientSets, nonChaosPods core_v1.PodList, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {
	annotatedPods := core_v1.PodList{}
	realPods := core_v1.PodList{}

	switch chaosDetails.AppDetail.AnnotationCheck {
	case true:
		for _, pod := range nonChaosPods.Items {
			isPodAnnotated, err := IsPodAnnotated(clients, pod, chaosDetails)
			if err != nil {
				return core_v1.PodList{}, err
			}
			if isPodAnnotated {
				annotatedPods.Items = append(annotatedPods.Items, pod)
			}
		}
		if len(annotatedPods.Items) == 0 {
			return annotatedPods, errors.Errorf("No annotated target pod found")
		}
	default:
		annotatedPods = nonChaosPods
	}

	newPodListLength := math.Maximum(1, math.Adjustment(podAffPerc, len(annotatedPods.Items)))
	rand.Seed(time.Now().UnixNano())

	// it will generate the random podlist
	// it starts from the random index and choose requirement no of pods next to that index in a circular way.
	index := rand.Intn(len(annotatedPods.Items))
	for i := 0; i < newPodListLength; i++ {
		realPods.Items = append(realPods.Items, annotatedPods.Items[index])
		index = (index + 1) % len(annotatedPods.Items)
	}
	return realPods, nil
}
