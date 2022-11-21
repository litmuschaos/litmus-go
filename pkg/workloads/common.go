package workloads

import (
	"context"
	"fmt"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Deployment            string = "Deployment"
	ReplicaSet            string = "ReplicaSet"
	DaemonSet             string = "DaemonSet"
	StatefulSet           string = "StatefulSet"
	DeploymentConfig      string = "DeploymentConfig"
	ReplicationController string = "ReplicationController"
	Rollout               string = "Rollout"
)

func getAllPods(namespace string, client clients.ClientSets) (*corev1.PodList, error) {
	return client.KubeClient.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{})
}

func getAllReplicaSets(namespace string, client clients.ClientSets) (*appsv1.ReplicaSetList, error) {
	return client.KubeClient.AppsV1().ReplicaSets(namespace).List(context.Background(), v1.ListOptions{})
}

func getAllReplicaControllers(namespace string, client clients.ClientSets) (*corev1.ReplicationControllerList, error) {
	return client.KubeClient.CoreV1().ReplicationControllers(namespace).List(context.Background(), v1.ListOptions{})
}

func getRSFromDeployment(rs *appsv1.ReplicaSetList, deployName string) []string {
	var result []string

	for _, r := range rs.Items {
		for _, owner := range r.OwnerReferences {
			if owner.Name == deployName && owner.Kind == Deployment {
				result = append(result, r.Name)
				break
			}
		}
	}

	return result
}

func getRCFromDeploymentConfig(rc *corev1.ReplicationControllerList, dcName string) []string {
	var result []string

	for _, r := range rc.Items {
		for _, owner := range r.OwnerReferences {
			if owner.Name == dcName && owner.Kind == DeploymentConfig {
				result = append(result, r.Name)
				break
			}
		}
	}

	return result
}

func getRSFromRollout(rs *appsv1.ReplicaSetList, roName string) []string {
	var result []string

	for _, r := range rs.Items {
		for _, owner := range r.OwnerReferences {
			if owner.Name == roName && owner.Kind == Rollout {
				result = append(result, r.Name)
				break
			}
		}
	}

	return result
}

func getPodsFromRS(pods *corev1.PodList, rsName string) []string {
	var result []string

	for _, p := range pods.Items {
		for _, owner := range p.OwnerReferences {
			if owner.Name == rsName && owner.Kind == ReplicaSet {
				result = append(result, p.Name)
				break
			}
		}
	}
	return result
}

func getPodsFromRC(pods *corev1.PodList, rcName string) []string {
	var result []string

	for _, p := range pods.Items {
		for _, owner := range p.OwnerReferences {
			if owner.Name == rcName && owner.Kind == ReplicationController {
				result = append(result, p.Name)
				break
			}
		}
	}
	return result
}

func GetPodsFromWorkload(appns, appkind, appName string, clients clients.ClientSets) ([]string, error) {

	w := NewWorkload(context.Background(), appns, clients)

	switch appkind {
	case "deployment":
		return w.GetPodsFromDeployment(appName)
	case "daemonset":
		return w.GetPodsFromDaemonSet(appName)
	case "statefulset":
		return w.GetPodsFromStatefulSet(appName)
	case "deploymentconfig":
		return w.GetPodsFromDeploymentConfig(appName)
	case "rollout":
		w.GetPodsFromRollout(appName)
	default:
		return nil, fmt.Errorf("unsupported appkind: %v", appkind)
	}
	return nil, nil
}
