package lib

import (
	"strings"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-autoscaler/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	retries "k8s.io/client-go/util/retry"

	"github.com/pkg/errors"
)

var (
	err                     error
	appsv1DeploymentClient  appsv1.DeploymentInterface
	appsv1StatefulsetClient appsv1.StatefulSetInterface
)

//PreparePodAutoscaler contains the prepration steps before chaos injection
func PreparePodAutoscaler(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// initialise the resource clients
	appsv1DeploymentClient = clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS)
	appsv1StatefulsetClient = clients.KubeClient.AppsV1().StatefulSets(experimentsDetails.AppNS)

	switch strings.ToLower(experimentsDetails.AppKind) {
	case "deployment", "deployments":

		appName, replicaCount, err := GetDeploymentDetails(experimentsDetails, clients)
		if err != nil {
			return errors.Errorf("Unable to get the name & replicaCount of the deployment, err: %v", err)
		}

		err = PodAutoscalerChaosInDeployment(experimentsDetails, clients, replicaCount, appName, resultDetails, eventsDetails, chaosDetails)
		if err != nil {
			return errors.Errorf("Unable to perform autoscaling, err: %v", err)
		}

		err = AutoscalerRecoveryInDeployment(experimentsDetails, clients, replicaCount, appName)
		if err != nil {
			return errors.Errorf("Unable to rollback the autoscaling, err: %v", err)
		}

	case "statefulset", "statefulsets":

		appName, replicaCount, err := GetStatefulsetDetails(experimentsDetails, clients)
		if err != nil {
			return errors.Errorf("Unable to get the name & replicaCount of the statefulset, err: %v", err)
		}

		err = PodAutoscalerChaosInStatefulset(experimentsDetails, clients, replicaCount, appName, resultDetails, eventsDetails, chaosDetails)
		if err != nil {
			return errors.Errorf("Unable to perform autoscaling, err: %v", err)
		}

		err = AutoscalerRecoveryInStatefulset(experimentsDetails, clients, replicaCount, appName)
		if err != nil {
			return errors.Errorf("Unable to rollback the autoscaling, err: %v", err)
		}

	default:
		return errors.Errorf("application type '%s' is not supported for the chaos", experimentsDetails.AppKind)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//GetDeploymentDetails is used to get the name and total number of replicas of the deployment
func GetDeploymentDetails(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, int, error) {

	var appReplica int
	var appName string

	deploymentList, err := appsv1DeploymentClient.List(metav1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(deploymentList.Items) == 0 {
		return "", 0, errors.Errorf("Unable to find the deployments with matching labels, err: %v", err)
	}
	for _, app := range deploymentList.Items {
		appReplica = int(*app.Spec.Replicas)
		appName = app.Name
	}

	return appName, appReplica, nil
}

//GetStatefulsetDetails is used to get the name and total number of replicas of the statefulsets
func GetStatefulsetDetails(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, int, error) {

	var appReplica int
	var appName string

	statefulsetList, err := appsv1StatefulsetClient.List(metav1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(statefulsetList.Items) == 0 {
		return "", 0, errors.Errorf("Unable to find the statefulsets with matching labels, err: %v", err)
	}
	for _, app := range statefulsetList.Items {
		appReplica = int(*app.Spec.Replicas)
		appName = app.Name
	}

	return appName, appReplica, nil
}

//PodAutoscalerChaosInDeployment scales up the replicas of deployment and verify the status
func PodAutoscalerChaosInDeployment(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, replicaCount int, appName string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Scale Application
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		appUnderTest, err := appsv1DeploymentClient.Get(appName, metav1.GetOptions{})
		if err != nil {
			return errors.Errorf("Failed to get latest version of Application Deployment, err: %v", err)
		}
		// modifying the replica count
		appUnderTest.Spec.Replicas = int32Ptr(int32(experimentsDetails.Replicas))
		_, updateErr := appsv1DeploymentClient.Update(appUnderTest)
		return updateErr
	})
	if retryErr != nil {
		return errors.Errorf("Unable to scale the deployment, err: %v", retryErr)
	}
	log.Info("Application Started Scaling")

	err = DeploymentStatusCheck(experimentsDetails, appName, clients, replicaCount, resultDetails, eventsDetails, chaosDetails)
	if err != nil {
		return errors.Errorf("Status Check failed, err: %v", err)
	}

	return nil
}

//PodAutoscalerChaosInStatefulset scales up the replicas of statefulset and verify the status
func PodAutoscalerChaosInStatefulset(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, replicaCount int, appName string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Scale Application
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		// Retrieve the latest version of Statefulset before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		appUnderTest, err := appsv1StatefulsetClient.Get(appName, metav1.GetOptions{})
		if err != nil {
			return errors.Errorf("Failed to get latest version of Application Statefulset, err: %v", err)
		}
		// modifying the replica count
		appUnderTest.Spec.Replicas = int32Ptr(int32(experimentsDetails.Replicas))
		_, updateErr := appsv1StatefulsetClient.Update(appUnderTest)
		return updateErr
	})
	if retryErr != nil {
		return errors.Errorf("Unable to scale the statefulset, err: %v", retryErr)
	}
	log.Info("Application Started Scaling")

	err = StatefulsetStatusCheck(experimentsDetails, appName, clients, replicaCount, resultDetails, eventsDetails, chaosDetails)
	if err != nil {
		return errors.Errorf("Status Check failed, err: %v", err)
	}

	return nil
}

// DeploymentStatusCheck check the status of deployment and verify the available replicas
func DeploymentStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets, replicaCount int, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	isFailed := false

	err = retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			deployment, err := appsv1DeploymentClient.Get(appName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to find the deployment with name %v, err: %v", appName, err)
			}
			log.Infof("Deployment's Available Replica Count is %v", deployment.Status.AvailableReplicas)
			if int(deployment.Status.AvailableReplicas) != experimentsDetails.Replicas {
				isFailed = true
				return errors.Errorf("Application is not scaled yet, err: %v", err)
			}
			isFailed = false
			return nil
		})

	if isFailed {
		err = AutoscalerRecoveryInDeployment(experimentsDetails, clients, replicaCount, appName)
		if err != nil {
			return errors.Errorf("Unable to perform autoscaling, err: %v", err)
		}
		return errors.Errorf("Failed to scale the application, err: %v", err)
	} else if err != nil {
		return err
	}

	// Keeping a wait time of 10s after all pod comes in running state
	// This is optional and used just for viewing the pod status
	time.Sleep(10 * time.Second)

	return nil
}

// StatefulsetStatusCheck check the status of statefulset and verify the available replicas
func StatefulsetStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets, replicaCount int, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	isFailed := false

	err = retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			statefulset, err := appsv1StatefulsetClient.Get(appName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to find the statefulset with name %v, err: %v", appName, err)
			}
			log.Infof("Statefulset's Ready Replica Count is: %v", statefulset.Status.ReadyReplicas)
			if int(statefulset.Status.ReadyReplicas) != experimentsDetails.Replicas {
				isFailed = true
				return errors.Errorf("Application is not scaled yet, err: %v", err)
			}
			isFailed = false
			return nil
		})

	if isFailed {
		err = AutoscalerRecoveryInStatefulset(experimentsDetails, clients, replicaCount, appName)
		if err != nil {
			return errors.Errorf("Unable to perform autoscaling, err: %v", err)
		}
		return errors.Errorf("Failed to scale the application, err: %v", err)
	} else if err != nil {
		return err
	}

	// Keeping a wait time of 10s after all pod comes in running state
	// This is optional and used just for viewing the pod status
	time.Sleep(10 * time.Second)

	return nil
}

//AutoscalerRecoveryInDeployment rollback the replicas to initial values in deployment
func AutoscalerRecoveryInDeployment(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, replicaCount int, appName string) error {

	// Scale back to initial number of replicas
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		appUnderTest, err := appsv1DeploymentClient.Get(appName, metav1.GetOptions{})
		if err != nil {
			return errors.Errorf("Failed to find the latest version of Application Deployment with name %v, err: %v", appName, err)
		}

		appUnderTest.Spec.Replicas = int32Ptr(int32(replicaCount)) // modify replica count
		_, updateErr := appsv1DeploymentClient.Update(appUnderTest)
		return updateErr
	})
	if retryErr != nil {
		return errors.Errorf("Unable to rollback the deployment, err: %v", retryErr)
	}
	log.Info("[Info]: Application pod started rolling back")

	err = retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			applicationDeploy, err := appsv1DeploymentClient.Get(appName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to find the deployment with name %v, err: %v", appName, err)
			}
			if int(applicationDeploy.Status.AvailableReplicas) != experimentsDetails.Replicas {
				log.Infof("Application Available Replica Count is: %v", applicationDeploy.Status.AvailableReplicas)
				return errors.Errorf("Unable to rollback to older replica count, err: %v", err)
			}
			return nil
		})

	if err != nil {
		return err
	}
	log.Info("[RollBack]: Application Pod roll back to initial number of replicas")

	return nil
}

//AutoscalerRecoveryInStatefulset rollback the replicas to initial values in deployment
func AutoscalerRecoveryInStatefulset(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, replicaCount int, appName string) error {

	// Scale back to initial number of replicas
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		// Retrieve the latest version of Statefulset before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		appUnderTest, err := appsv1StatefulsetClient.Get(appName, metav1.GetOptions{})
		if err != nil {
			return errors.Errorf("Failed to find the latest version of Statefulset with name %v, err: %v", appName, err)
		}

		appUnderTest.Spec.Replicas = int32Ptr(int32(replicaCount)) // modify replica count
		_, updateErr := appsv1StatefulsetClient.Update(appUnderTest)
		return updateErr
	})
	if retryErr != nil {
		return errors.Errorf("Unable to rollback the statefulset, err: %v", retryErr)
	}
	log.Info("[Info]: Application pod started rolling back")

	err = retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			applicationDeploy, err := appsv1StatefulsetClient.Get(appName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to find the statefulset with name %v, err: %v", appName, err)
			}
			if int(applicationDeploy.Status.ReadyReplicas) != experimentsDetails.Replicas {
				log.Infof("Application Ready Replica Count is: %v", applicationDeploy.Status.ReadyReplicas)
				return errors.Errorf("Unable to roll back to older replica count, err: %v", err)
			}
			return nil
		})

	if err != nil {
		return err
	}
	log.Info("[RollBack]: Application Pod roll back to initial number of replicas")

	return nil
}

func int32Ptr(i int32) *int32 { return &i }
