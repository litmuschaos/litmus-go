package annotation

import (
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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
								log.Infof("[Info]: chaos candidate of kind: %v, name: %v, namespace: %v", chaosDetails.AppDetail.Kind, deploy.Name, deploy.Namespace)
								return true, nil
							}
						}
					}
				}
			}
		}
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
						log.Infof("[Info]: chaos candidate of kind: %v, name: %v, namespace: %v", chaosDetails.AppDetail.Kind, sts.Name, sts.Namespace)
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
						log.Infof("[Info]: chaos candidate of kind: %v, name: %v, namespace: %v", chaosDetails.AppDetail.Kind, ds.Name, ds.Namespace)
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
			return false, errors.Errorf("No deploymentconfig found with matching labels, err: %v", err)
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
								log.Infof("[Info]: chaos candidate of kind: %v, name: %v, namespace: %v", chaosDetails.AppDetail.Kind, dc.GetName(), dc.GetNamespace())
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
								log.Infof("[Info]: chaos candidate of kind: %v, name: %v, namespace: %v", chaosDetails.AppDetail.Kind, ro.GetName(), ro.GetNamespace())
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
