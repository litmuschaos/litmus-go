package workloads

import (
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	core_v1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getPodParent(targetPod core_v1.Pod, clients clients.ClientSets) (string, string, error) {
	for _, own := range targetPod.OwnerReferences {
		switch own.Kind {
		case "ReplicaSet":
			return getRSParent(own.Name, targetPod.Namespace, clients)
		case "StatefulSet":
			return own.Name, own.Kind, nil
		case "DaemonSet":
			return own.Name, own.Kind, nil
		case "ReplicationController":
			return getRCParent(own.Name, targetPod.Namespace, clients)
		}
	}
	return "", "", fmt.Errorf("no parent found for %v pod", targetPod.Name)
}

func getRSParent(name, ns string, clients clients.ClientSets) (string, string, error) {
	rs, err := clients.KubeClient.AppsV1().ReplicaSets(ns).Get(context.Background(), name, v1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	for _, own := range rs.OwnerReferences {
		switch own.Kind {
		case "Deployment":
			return own.Name, own.Kind, nil
		case "Rollout":
			return own.Name, own.Kind, nil
		}
	}
	return "", "", fmt.Errorf("no parent found for %v rs", name)
}

func getRCParent(name, ns string, clients clients.ClientSets) (string, string, error) {
	rc, err := clients.KubeClient.CoreV1().ReplicationControllers(ns).Get(context.Background(), name, v1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	for _, own := range rc.OwnerReferences {
		switch own.Kind {
		case "DeploymentConfig":
			return own.Name, own.Kind, nil
		}
	}
	return "", "", fmt.Errorf("no parent found for %v rc", name)
}

// GetParentNameAndKind derive the parent name of the given target pod
func GetParentNameAndKind(clients clients.ClientSets, targetPod core_v1.Pod) (string, string, error) {
	return getPodParent(targetPod, clients)
}
