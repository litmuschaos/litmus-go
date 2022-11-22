package common

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/litmuschaos/litmus-go/pkg/workloads"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	core_v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//DeletePod deletes the specified pod and wait until it got terminated
func DeletePod(podName, podLabel, namespace string, timeout, delay int, clients clients.ClientSets) error {

	if err := clients.KubeClient.CoreV1().Pods(namespace).Delete(context.Background(), podName, v1.DeleteOptions{}); err != nil {
		return err
	}

	// waiting for the termination of the pod
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{LabelSelector: podLabel})
			if err != nil {
				return errors.Errorf("Unable to delete the pod, err: %v", err)
			} else if len(podSpec.Items) != 0 {
				return errors.Errorf("Unable to delete the pod")
			}
			return nil
		})
}

//DeleteAllPod deletes all the pods with matching labels and wait until all the pods got terminated
func DeleteAllPod(podLabel, namespace string, timeout, delay int, clients clients.ClientSets) error {

	if err := clients.KubeClient.CoreV1().Pods(namespace).DeleteCollection(context.Background(), v1.DeleteOptions{}, v1.ListOptions{LabelSelector: podLabel}); err != nil {
		return err
	}

	// waiting for the termination of the pod
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{LabelSelector: podLabel})
			if err != nil {
				return errors.Errorf("Unable to delete the pods, err: %v", err)
			} else if len(podSpec.Items) != 0 {
				return errors.Errorf("Unable to delete the pods")
			}
			return nil
		})
}

// getChaosPodResourceRequirements will return the resource requirements on chaos pod
func getChaosPodResourceRequirements(podName, containerName, namespace string, clients clients.ClientSets) (core_v1.ResourceRequirements, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(namespace).Get(context.Background(), podName, v1.GetOptions{})
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

// SetHelperData derive the data from experiment pod and sets into experimentDetails struct
// which can be used to create helper pod
func SetHelperData(chaosDetails *types.ChaosDetails, setHelperData string, clients clients.ClientSets) error {
	var pod *core_v1.Pod
	pod, err = clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).Get(context.Background(), chaosDetails.ChaosPodName, v1.GetOptions{})
	if err != nil {
		return err
	}

	// Get Labels
	labels := pod.ObjectMeta.Labels
	delete(labels, "controller-uid")
	delete(labels, "job-name")
	chaosDetails.Labels = labels

	switch setHelperData {
	case "false":
		return nil

	default:

		// Get Chaos Pod Annotation
		chaosDetails.Annotations = pod.Annotations

		// Get ImagePullSecrets
		chaosDetails.ImagePullSecrets = pod.Spec.ImagePullSecrets

		// Get Resource Requirements
		chaosDetails.Resources, err = getChaosPodResourceRequirements(chaosDetails.ChaosPodName, chaosDetails.ExperimentName, chaosDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("unable to get resource requirements, err: %v", err)
		}
		return nil
	}
}

// GetHelperLabels return the labels of the helper pod
func GetHelperLabels(labels map[string]string, runID, experimentName string) map[string]string {
	labels["app"] = experimentName + "-helper-" + runID
	return labels
}

// VerifyExistanceOfPods check the availibility of list of pods
func VerifyExistanceOfPods(namespace, pods string, clients clients.ClientSets) (bool, error) {

	if strings.TrimSpace(pods) == "" {
		return false, nil
	}

	podList := strings.Split(strings.TrimSpace(pods), ",")
	for index := range podList {
		isPodsAvailable, err := CheckForAvailibiltyOfPod(namespace, podList[index], clients)
		if err != nil {
			return false, err
		}
		if !isPodsAvailable {
			return isPodsAvailable, errors.Errorf("%v pod is not available in %v namespace", podList[index], namespace)
		}
	}
	return true, nil
}

//GetPodList check for the availability of the target pod for the chaos execution
// if the target pod is not defined it will derive the random target pod list using pod affected percentage
func GetPodList(targetPods string, podAffPerc int, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {
	finalPods := core_v1.PodList{}
	var namespace string
	if chaosDetails.AppDetail != nil {
		namespace = chaosDetails.AppDetail[0].Namespace
	} else {
		namespace = chaosDetails.ChaosNamespace
	}

	isPodsAvailable, err := VerifyExistanceOfPods(namespace, targetPods, clients)
	if err != nil {
		return core_v1.PodList{}, err
	}

	// getting the pod, if the target pods is defined
	// else select a random target pod from the specified labels
	switch isPodsAvailable {
	case true:
		podList, err := GetTargetPodsWhenTargetPodsENVSet(targetPods, namespace, clients, chaosDetails)
		if err != nil {
			return core_v1.PodList{}, err
		}
		finalPods.Items = append(finalPods.Items, podList.Items...)
	default:
		podList, err := GetTargetPodsWhenTargetPodsENVNotSet(podAffPerc, clients, chaosDetails)
		if err != nil {
			return core_v1.PodList{}, err
		}
		finalPods.Items = append(finalPods.Items, podList.Items...)
	}
	return finalPods, nil
}

// CheckForAvailibiltyOfPod check the availibility of the specified pod
func CheckForAvailibiltyOfPod(namespace, name string, clients clients.ClientSets) (bool, error) {

	if name == "" {
		return false, nil
	}
	_, err := clients.KubeClient.CoreV1().Pods(namespace).Get(context.Background(), name, v1.GetOptions{})

	if err != nil && k8serrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

//FilterNonChaosPods remove the chaos pods(operator, runner) for the podList
// it filter when the applabels are not defined and it will select random pods from appns
func FilterNonChaosPods(ns, labels string, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(ns).List(context.Background(), v1.ListOptions{LabelSelector: labels})
	if err != nil {
		return core_v1.PodList{}, err
	} else if len(podList.Items) == 0 {
		return core_v1.PodList{}, errors.Wrapf(err, "Failed to find the pod with matching labels in %v namespace", ns)
	}
	nonChaosPods := core_v1.PodList{}
	// ignore chaos pods
	for index, pod := range podList.Items {
		if pod.Labels["chaosUID"] == "" && pod.Labels["name"] != "chaos-operator" {
			nonChaosPods.Items = append(nonChaosPods.Items, podList.Items[index])
		}
	}
	return nonChaosPods, nil
}

// GetTargetPodsWhenTargetPodsENVSet derive the specific target pods, if TARGET_PODS env is set
func GetTargetPodsWhenTargetPodsENVSet(targetPods, namespace string, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {
	targetPodsList := strings.Split(targetPods, ",")
	realPods := core_v1.PodList{}

	for index := range targetPodsList {
		pod, err := clients.KubeClient.CoreV1().Pods(namespace).Get(context.Background(), strings.TrimSpace(targetPodsList[index]), v1.GetOptions{})
		if err != nil {
			return core_v1.PodList{}, errors.Wrapf(err, "Failed to get %v pod in %v namespace", targetPodsList[index], namespace)
		}
		realPods.Items = append(realPods.Items, *pod)
	}
	return realPods, nil
}

// SetTargets set the target details in chaosdetails struct
func SetTargets(target, chaosStatus, kind string, chaosDetails *types.ChaosDetails) {

	for i := range chaosDetails.Targets {
		if chaosDetails.Targets[i].Name == target {
			chaosDetails.Targets[i].ChaosStatus = chaosStatus
			return
		}
	}
	newTarget := v1alpha1.TargetDetails{
		Name:        target,
		Kind:        kind,
		ChaosStatus: chaosStatus,
	}
	chaosDetails.Targets = append(chaosDetails.Targets, newTarget)
}

// SetParentName set the parent name in chaosdetails struct
func SetParentName(parentName, kind, ns string, chaosDetails *types.ChaosDetails) {
	parent := types.ParentResource{Name: parentName, Kind: kind, Namespace: ns}

	if chaosDetails.ParentsResources == nil {
		chaosDetails.ParentsResources = []types.ParentResource{parent}
	} else {
		for i := range chaosDetails.ParentsResources {
			if chaosDetails.ParentsResources[i].Name == parentName {
				return
			}
		}
		chaosDetails.ParentsResources = append(chaosDetails.ParentsResources, parent)
	}
}

// GetTargetPodsWhenTargetPodsENVNotSet derives the random target pod list, if TARGET_PODS env is not set
func GetTargetPodsWhenTargetPodsENVNotSet(podAffPerc int, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {

	var finalPods core_v1.PodList
	var podKind = false
	if len(chaosDetails.AppDetail) == 1 && chaosDetails.AppDetail[0].Kind == "KIND" {
		// select random pod from ns
		pods, err := FilterNonChaosPods(chaosDetails.AppDetail[0].Namespace, "", clients, chaosDetails)
		if err != nil {
			return finalPods, err
		}
		return filterPodsByPercentage(pods, podAffPerc), nil
	}

	for _, target := range chaosDetails.AppDetail {
		switch target.Kind {
		case "pod":
			for _, name := range target.Names {
				pod, err := clients.KubeClient.CoreV1().Pods(target.Namespace).Get(context.Background(), name, v1.GetOptions{})
				if err != nil {
					return finalPods, err
				}
				finalPods.Items = append(finalPods.Items, *pod)
			}
			podKind = true
		default:
			if target.Names != nil {
				pods, err := workloads.GetPodsFromWorkloads(target, clients)
				if err != nil {
					return finalPods, err
				}
				finalPods.Items = append(finalPods.Items, pods.Items...)
			} else {
				for _, label := range target.Labels {
					pods, err := clients.KubeClient.CoreV1().Pods(target.Namespace).List(context.Background(), v1.ListOptions{LabelSelector: label})
					if err != nil {
						return finalPods, err
					}
					finalPods.Items = append(finalPods.Items, pods.Items...)
				}
			}
		}
	}

	if len(finalPods.Items) == 0 {
		return finalPods, errors.Errorf("No target pod found")
	}

	if podKind {
		return finalPods, nil
	}
	return filterPodsByPercentage(finalPods, podAffPerc), nil
}

func filterPodsByPercentage(finalPods core_v1.PodList, podAffPerc int) core_v1.PodList {
	finalPods = removeDuplicatePods(finalPods)

	newPodListLength := math.Maximum(1, math.Adjustment(math.Minimum(podAffPerc, 100), len(finalPods.Items)))
	rand.Seed(time.Now().UnixNano())

	var realPods core_v1.PodList
	// it will generate the random podlist
	// it starts from the random index and choose requirement no of pods next to that index in a circular way.
	index := rand.Intn(len(finalPods.Items))
	for i := 0; i < newPodListLength; i++ {
		realPods.Items = append(realPods.Items, finalPods.Items[index])
		index = (index + 1) % len(finalPods.Items)
	}
	return realPods
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
	pod, err := clients.KubeClient.CoreV1().Pods(chaosNamespace).Get(context.Background(), chaosPodName, v1.GetOptions{})
	if err != nil {
		return "", err
	}
	return pod.Spec.ServiceAccountName, nil
}

//GetTargetContainer will fetch the container name from application pod
//This container will be used as target container
func GetTargetContainer(appNamespace, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(appNamespace).Get(context.Background(), appName, v1.GetOptions{})
	if err != nil {
		return "", err
	}
	return pod.Spec.Containers[0].Name, nil
}

//GetContainerID  derive the container id of the application container
func GetContainerID(appNamespace, targetPod, targetContainer string, clients clients.ClientSets) (string, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(appNamespace).Get(context.Background(), targetPod, v1.GetOptions{})
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
	return containerID, nil
}

//GetRuntimeBasedContainerID extract out the container id of the target container based on the container runtime
func GetRuntimeBasedContainerID(containerRuntime, socketPath, targetPods, appNamespace, targetContainer string, clients clients.ClientSets) (string, error) {

	var containerID string
	switch containerRuntime {
	case "docker":
		host := "unix://" + socketPath
		// deriving the container id of the pause container
		cmd := "sudo docker --host " + host + " ps | grep k8s_POD_" + targetPods + "_" + appNamespace + " | awk '{print $1}'"
		out, err := exec.Command("/bin/sh", "-c", cmd).CombinedOutput()
		if err != nil {
			log.Errorf("[docker]: Failed to run docker ps command: %s", string(out))
			return "", err
		}
		containerID = strings.TrimSpace(string(out))
	case "containerd", "crio":
		containerID, err = GetContainerID(appNamespace, targetPods, targetContainer, clients)
		if err != nil {
			return "", err
		}
	default:
		return "", errors.Errorf("%v container runtime not suported", containerRuntime)
	}
	log.Infof("Container ID: %v", containerID)

	return containerID, nil
}

// CheckContainerStatus checks the status of the application container
func CheckContainerStatus(appNamespace, appName string, timeout, delay int, clients clients.ClientSets) error {
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(appNamespace).Get(context.Background(), appName, v1.GetOptions{})
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

// GetPodListFromSpecifiedNodes will filter out the pod list scheduled on specified node
func GetPodListFromSpecifiedNodes(podAffPerc int, nodeLabel string, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {
	var nodes *core_v1.NodeList

	// identify node list from the provided node label
	nodes, err = clients.KubeClient.CoreV1().Nodes().List(context.Background(), v1.ListOptions{LabelSelector: nodeLabel})
	if err != nil {
		return core_v1.PodList{}, errors.Errorf("Failed to find the nodes with matching label, err: %v", err)
	} else if len(nodes.Items) == 0 {
		return core_v1.PodList{}, errors.Errorf("Failed to find the nodes with matching label")
	}
	nodeNames := []string{}
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	log.Infof("[Chaos]:Looking for pods with specified attributes on nodes targeted: %v", nodeNames)

	pods, err := GetPodList("", 100, clients, chaosDetails)
	if err != nil {
		return core_v1.PodList{}, err
	}

	return getTargetPodsWhenNodeFilterSet(podAffPerc, pods, nodeNames)
}

// getTargetPodsWhenNodeFilterSet will give the target pod when the node filter is setup
func getTargetPodsWhenNodeFilterSet(podAffPerc int, pods core_v1.PodList, nodes []string) (core_v1.PodList, error) {
	nodeFilteredPods := core_v1.PodList{}

	// add filter for pods which are on derived node list
	for _, pod := range pods.Items {
		for _, itemIndex := range nodes {
			if pod.Spec.NodeName == itemIndex {
				nodeFilteredPods.Items = append(nodeFilteredPods.Items, pod)
				break // a given pod obj is found in only one node, break and start checking next pod
			}

		}
	}

	if len(nodeFilteredPods.Items) == 0 {
		return nodeFilteredPods, errors.Errorf("No pod found with desired attributes on specified node(s)")
	}

	return filterPodsByPercentage(nodeFilteredPods, podAffPerc), nil
}

func GetTargetPods(nodeLabel, targetPods, podsAffectedPerc string, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (core_v1.PodList, error) {

	podAffectedPerc, _ := strconv.Atoi(podsAffectedPerc)

	var pods core_v1.PodList

	if nodeLabel != "" && targetPods == "" {
		pods, err = GetPodListFromSpecifiedNodes(podAffectedPerc, nodeLabel, clients, chaosDetails)
		if err != nil {
			return core_v1.PodList{}, err
		}
	} else {
		if targetPods != "" && nodeLabel != "" {
			log.Infof("TARGET_PODS env is provided, overriding the NODE_LABEL input")
		}
		pods, err = GetPodList(targetPods, podAffectedPerc, clients, chaosDetails)
		if err != nil {
			return core_v1.PodList{}, err
		}
	}

	podNames := []string{}
	for _, pod := range pods.Items {
		podNames = append(podNames, pod.Name)
	}
	log.Infof("[Chaos]:Number of pods targeted: %v", len(pods.Items))
	log.Infof("Target pods list for chaos, %v", podNames)

	return pods, nil
}

func removeDuplicatePods(pods core_v1.PodList) core_v1.PodList {

	var unique core_v1.PodList

	for _, pod := range pods.Items {
		found := false
		for _, v := range unique.Items {
			if pod.Name == v.Name {
				found = true
				break
			}
		}
		if !found {
			unique.Items = append(unique.Items, pod)
		}
	}
	return unique
}

func FilterPodsForNodes(targetPodList core_v1.PodList, containerName string) map[string]*TargetsDetails {
	targets := make(map[string]*TargetsDetails)

	for _, pod := range targetPodList.Items {

		td := target{
			Name:            pod.Name,
			Namespace:       pod.Namespace,
			TargetContainer: containerName,
		}

		if td.TargetContainer == "" {
			td.TargetContainer = pod.Spec.Containers[0].Name
		}

		if targets[pod.Spec.NodeName] == nil {
			targets[pod.Spec.NodeName] = &TargetsDetails{
				Target: []target{td},
			}
		} else {
			targets[pod.Spec.NodeName].Target = append(targets[pod.Spec.NodeName].Target, td)
		}
	}
	return targets
}

type TargetsDetails struct {
	Target []target
}

type target struct {
	Namespace       string
	Name            string
	TargetContainer string
}

func ParseTargets() (*TargetsDetails, error) {
	var targets TargetsDetails
	targetEnv := os.Getenv("TARGETS")
	if targetEnv == "" {
		return nil, fmt.Errorf("no target found, provide atleast one target")
	}

	for _, t := range strings.Split(targetEnv, ";") {
		targetList := strings.Split(t, ":")
		if len(targetList) != 3 {
			return nil, fmt.Errorf("unsupported target: '%v', provide target in '<name>:<namespace>:<containerName>", targetList)
		}
		targets.Target = append(targets.Target, target{
			Name:            targetList[0],
			Namespace:       targetList[1],
			TargetContainer: targetList[2],
		})
	}
	return &targets, nil
}
