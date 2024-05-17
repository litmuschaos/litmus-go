// Package workloads implements utility to derive the pods from the parent workloads
package workloads

import (
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/palantir/stacktrace"
	"strings"

	kcorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type Workload struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
}

var (
	gvrrc = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "replicacontrollers",
	}

	gvrrs = schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "replicasets",
	}
)

// GetPodsFromWorkloads derives the pods from the parent workloads
func GetPodsFromWorkloads(target types.AppDetails, client clients.ClientSets) (kcorev1.PodList, error) {

	allPods, err := getAllPods(target.Namespace, client)
	if err != nil {
		return kcorev1.PodList{}, stacktrace.Propagate(err, "could not get all pods")
	}
	return getPodsFromWorkload(target, allPods, client.DynamicClient)
}

func getPodsFromWorkload(target types.AppDetails, allPods *kcorev1.PodList, dynamicClient dynamic.Interface) (kcorev1.PodList, error) {
	var pods kcorev1.PodList
	for _, wld := range target.Names {
		found := false
		for _, r := range allPods.Items {
			ownerType, ownerName, err := GetPodOwnerTypeAndName(&r, dynamicClient)
			if err != nil {
				return pods, err
			}
			if ownerName == "" || ownerType == "" {
				continue
			}
			if target.Kind == ownerType && wld == ownerName {
				found = true
				pods.Items = append(pods.Items, r)
			}
		}
		if !found {
			return pods, cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{namespace: %s, kind: %s, name: %s}", target.Namespace, target.Kind, wld), Reason: "no pod found for specified target"}
		}
	}
	return pods, nil
}

func GetPodOwnerTypeAndName(pod *kcorev1.Pod, dynamicClient dynamic.Interface) (parentType, parentName string, err error) {
	for _, owner := range pod.GetOwnerReferences() {
		parentName = owner.Name
		if owner.Kind == "StatefulSet" || owner.Kind == "DaemonSet" {
			return strings.ToLower(owner.Kind), parentName, nil
		}

		if owner.Kind == "ReplicaSet" && strings.HasSuffix(owner.Name, pod.Labels["pod-template-hash"]) {
			return getParent(owner.Name, pod.Namespace, gvrrs, dynamicClient)
		}

		if owner.Kind == "ReplicaController" {
			return getParent(owner.Name, pod.Namespace, gvrrc, dynamicClient)
		}
	}
	return parentType, parentName, nil
}

func getParent(name, namespace string, gvr schema.GroupVersionResource, dynamicClient dynamic.Interface) (string, string, error) {
	res, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(context.Background(), name, v1.GetOptions{})
	if err != nil {
		return "", "", cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{namespace: %s, kind: %s, name: %s}", namespace, gvr.Resource, name), Reason: err.Error()}
	}

	for _, v := range res.GetOwnerReferences() {
		kind := strings.ToLower(v.Kind)
		if kind == "deployment" || kind == "rollout" || kind == "deploymentconfig" {
			return kind, v.Name, nil
		}
	}
	return "", "", nil
}

func getAllPods(namespace string, client clients.ClientSets) (*kcorev1.PodList, error) {
	pods, err := client.KubeClient.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{namespace: %s, resource: AllPods}", namespace), Reason: fmt.Sprintf("failed to get all pods :%s", err.Error())}
	}
	return pods, nil
}
