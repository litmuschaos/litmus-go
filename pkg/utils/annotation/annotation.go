package annotation

import (
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// IsParentAnnotated check whether the target pod's parent is annotated or not
func IsParentAnnotated(clients clients.ClientSets, parentName string, chaosDetails *types.ChaosDetails) (bool, error) {

	switch chaosDetails.AppDetail.Kind {
	case "deployment", "deployments":
		deploy, err := clients.KubeClient.AppsV1().Deployments(chaosDetails.AppDetail.Namespace).Get(parentName, v1.GetOptions{})
		if err != nil {
			return false, err
		}
		if deploy.ObjectMeta.Annotations[chaosDetails.AppDetail.AnnotationKey] == chaosDetails.AppDetail.AnnotationValue {
			return true, nil
		}
	case "statefulset", "statefulsets":
		sts, err := clients.KubeClient.AppsV1().StatefulSets(chaosDetails.AppDetail.Namespace).Get(parentName, v1.GetOptions{})
		if err != nil {
			return false, err
		}
		if sts.ObjectMeta.Annotations[chaosDetails.AppDetail.AnnotationKey] == chaosDetails.AppDetail.AnnotationValue {
			return true, nil
		}
	case "daemonset", "daemonsets":
		ds, err := clients.KubeClient.AppsV1().DaemonSets(chaosDetails.AppDetail.Namespace).Get(parentName, v1.GetOptions{})
		if err != nil {
			return false, err
		}
		if ds.ObjectMeta.Annotations[chaosDetails.AppDetail.AnnotationKey] == chaosDetails.AppDetail.AnnotationValue {
			return true, nil
		}
	case "deploymentconfig", "deploymentconfigs":
		gvrdc := schema.GroupVersionResource{
			Group:    "apps.openshift.io",
			Version:  "v1",
			Resource: "deploymentconfigs",
		}
		dc, err := clients.DynamicClient.Resource(gvrdc).Namespace(chaosDetails.AppDetail.Namespace).Get(parentName, v1.GetOptions{})
		if err != nil {
			return false, err
		}
		annotations := dc.GetAnnotations()
		if annotations[chaosDetails.AppDetail.AnnotationKey] == chaosDetails.AppDetail.AnnotationValue {
			return true, nil
		}
	case "rollout", "rollouts":
		gvrro := schema.GroupVersionResource{
			Group:    "argoproj.io",
			Version:  "v1alpha1",
			Resource: "rollouts",
		}
		ro, err := clients.DynamicClient.Resource(gvrro).Namespace(chaosDetails.AppDetail.Namespace).Get(parentName, v1.GetOptions{})
		if err != nil {
			return false, err
		}
		annotations := ro.GetAnnotations()
		if annotations[chaosDetails.AppDetail.AnnotationKey] == chaosDetails.AppDetail.AnnotationValue {
			return true, nil
		}
	default:
		return false, errors.Errorf("%v appkind is not supported", chaosDetails.AppDetail.Kind)
	}
	return false, nil
}

// GetParentName derive the parent name of the given target pod
func GetParentName(clients clients.ClientSets, targetPod core_v1.Pod, chaosDetails *types.ChaosDetails) (string, error) {

	switch chaosDetails.AppDetail.Kind {
	case "deployment", "deployments":
		return getDeploymentName(clients, targetPod, chaosDetails)
	case "statefulset", "statefulsets":
		return getStatefulsetName(clients, targetPod, chaosDetails)
	case "daemonset", "daemonsets":
		return getDaemonsetName(clients, targetPod, chaosDetails)
	case "deploymentconfig", "deploymentconfigs":
		return getDeploymentConfigName(clients, targetPod, chaosDetails)
	case "rollout", "rollouts":
		return getRolloutName(clients, targetPod, chaosDetails)
	default:
		return "", errors.Errorf("%v appkind is not supported", chaosDetails.AppDetail.Kind)
	}
}

// getDeploymentName derive the deployment name belongs to the given target pod
// it extract the parent name from the owner references
func getDeploymentName(clients clients.ClientSets, targetPod core_v1.Pod, chaosDetails *types.ChaosDetails) (string, error) {

	deployList, err := clients.KubeClient.AppsV1().Deployments(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
	if err != nil || len(deployList.Items) == 0 {
		return "", errors.Errorf("No deployment found with matching label, err: %v", err)
	}
	rsOwnerRef := targetPod.OwnerReferences
	for _, deploy := range deployList.Items {
		for _, own := range rsOwnerRef {
			if own.Kind == "ReplicaSet" {
				rs, err := clients.KubeClient.AppsV1().ReplicaSets(chaosDetails.AppDetail.Namespace).Get(own.Name, v1.GetOptions{})
				if err != nil {
					return "", err
				}
				ownerRef := rs.OwnerReferences
				for _, own := range ownerRef {
					if own.Kind == "Deployment" && own.Name == deploy.Name {
						return deploy.Name, nil
					}
				}
			}
		}
	}
	return "", errors.Errorf("no deployment found for %v pod", targetPod.Name)
}

// getStatefulsetName derive the statefulset name belongs to the given target pod
// it extract the parent name from the owner references
func getStatefulsetName(clients clients.ClientSets, targetPod core_v1.Pod, chaosDetails *types.ChaosDetails) (string, error) {

	stsList, err := clients.KubeClient.AppsV1().StatefulSets(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
	if err != nil || len(stsList.Items) == 0 {
		return "", errors.Errorf("No statefulset found with matching label, err: %v", err)
	}
	for _, sts := range stsList.Items {
		ownerRef := targetPod.OwnerReferences
		for _, own := range ownerRef {
			if own.Kind == "StatefulSet" && own.Name == sts.Name {
				return sts.Name, nil
			}
		}
	}
	return "", errors.Errorf("no statefulset found for %v pod", targetPod.Name)
}

// getDaemonsetName derive the daemonset name belongs to the given target pod
// it extract the parent name from the owner references
func getDaemonsetName(clients clients.ClientSets, targetPod core_v1.Pod, chaosDetails *types.ChaosDetails) (string, error) {

	dsList, err := clients.KubeClient.AppsV1().DaemonSets(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
	if err != nil || len(dsList.Items) == 0 {
		return "", errors.Errorf("No daemonset found with matching label, err: %v", err)
	}
	for _, ds := range dsList.Items {
		ownerRef := targetPod.OwnerReferences
		for _, own := range ownerRef {
			if own.Kind == "DaemonSet" && own.Name == ds.Name {
				return ds.Name, nil
			}
		}
	}
	return "", errors.Errorf("no daemonset found for %v pod", targetPod.Name)
}

// getDeploymentConfigName derive the deploymentConfig name belongs to the given target pod
// it extract the parent name from the owner references
func getDeploymentConfigName(clients clients.ClientSets, targetPod core_v1.Pod, chaosDetails *types.ChaosDetails) (string, error) {

	gvrdc := schema.GroupVersionResource{
		Group:    "apps.openshift.io",
		Version:  "v1",
		Resource: "deploymentconfigs",
	}
	deploymentConfigList, err := clients.DynamicClient.Resource(gvrdc).Namespace(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
	if err != nil || len(deploymentConfigList.Items) == 0 {
		return "", errors.Errorf("No deploymentconfig found with matching labels, err: %v", err)
	}
	for _, dc := range deploymentConfigList.Items {
		rcOwnerRef := targetPod.OwnerReferences
		for _, own := range rcOwnerRef {
			if own.Kind == "ReplicationController" {
				rc, err := clients.KubeClient.CoreV1().ReplicationControllers(chaosDetails.AppDetail.Namespace).Get(own.Name, v1.GetOptions{})
				if err != nil {
					return "", err
				}
				ownerRef := rc.OwnerReferences
				for _, own := range ownerRef {
					if own.Kind == "DeploymentConfig" && own.Name == dc.GetName() {
						return dc.GetName(), nil
					}
				}
			}
		}
	}
	return "", errors.Errorf("no deploymentConfig found for %v pod", targetPod.Name)
}

// getDeploymentConfigName derive the rollout name belongs to the given target pod
// it extract the parent name from the owner references
func getRolloutName(clients clients.ClientSets, targetPod core_v1.Pod, chaosDetails *types.ChaosDetails) (string, error) {

	gvrro := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "rollouts",
	}
	rolloutList, err := clients.DynamicClient.Resource(gvrro).Namespace(chaosDetails.AppDetail.Namespace).List(v1.ListOptions{LabelSelector: chaosDetails.AppDetail.Label})
	if err != nil || len(rolloutList.Items) == 0 {
		return "", errors.Errorf("No rollouts found with matching labels, err: %v", err)
	}
	for _, ro := range rolloutList.Items {
		rsOwnerRef := targetPod.OwnerReferences
		for _, own := range rsOwnerRef {
			if own.Kind == "ReplicaSet" {
				rs, err := clients.KubeClient.AppsV1().ReplicaSets(chaosDetails.AppDetail.Namespace).Get(own.Name, v1.GetOptions{})
				if err != nil {
					return "", err
				}
				ownerRef := rs.OwnerReferences
				for _, own := range ownerRef {
					if own.Kind == "Rollout" && own.Name == ro.GetName() {
						return ro.GetName(), nil
					}
				}
			}
		}
	}
	return "", errors.Errorf("no rollout found for %v pod", targetPod.Name)
}
