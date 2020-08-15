package pod_autoscaler

import (
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-autoscaler/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	retries "k8s.io/client-go/util/retry"

	"github.com/pkg/errors"
)

var err error

//PreparePodAutoscaler contains the prepration steps before chaos injection
func PreparePodAutoscaler(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	appName, replicaCount, err := GetApplicationDetails(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the relicaCount of the application, err: %v", err)
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}

	err = PodAutoscalerChaos(experimentsDetails, clients, replicaCount, appName, resultDetails, eventsDetails, chaosDetails)

	if err != nil {
		return errors.Errorf("Unable to perform autoscaling, due to %v", err)
	}

	err = AutoscalerRecovery(experimentsDetails, clients, replicaCount, appName)
	if err != nil {
		return errors.Errorf("Unable to perform autoscaling, due to %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}
	return nil
}

//waitForRampTime waits for the given ramp time duration (in seconds)
func waitForRampTime(experimentsDetails *experimentTypes.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.RampTime) * time.Second)
}

//GetApplicationDetails is used to get the application name, replicas of the application
func GetApplicationDetails(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, int, error) {

	var appReplica int
	var appName string
	// Get Deployment replica count
	applicationList, err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS).List(metav1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(applicationList.Items) == 0 {
		return "", 0, errors.Errorf("Unable to get application, err: %v", err)
	}
	for _, app := range applicationList.Items {
		appReplica = int(*app.Spec.Replicas)
		appName = app.Name

	}
	return appName, appReplica, nil

}

//PodAutoscalerChaos scales up the application pod replicas
func PodAutoscalerChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, replicaCount int, appName string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	applicationClient := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS)

	replicas := int32(experimentsDetails.Replicas)
	// Scale Application
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		appUnderTest, err := applicationClient.Get(appName, metav1.GetOptions{})
		if err != nil {
			return errors.Errorf("Failed to get latest version of Application Deployment: %v", err)
		}

		appUnderTest.Spec.Replicas = int32Ptr(replicas) // modify replica count
		_, updateErr := applicationClient.Update(appUnderTest)
		return updateErr
	})
	if retryErr != nil {
		return errors.Errorf("Unable to scale the application, due to: %v", retryErr)
	}
	log.Info("Application Started Scaling")

	err = ApplicationPodStatusCheck(experimentsDetails, appName, clients, replicaCount, resultDetails, eventsDetails, chaosDetails)
	if err != nil {
		return errors.Errorf("Status Check failed, err: %v", err)
	}

	return nil
}

// ApplicationPodStatusCheck checks the status of the application pod
func ApplicationPodStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets, replicaCount int, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now().Unix()
	failFlag := false
	applicationClient := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS)
	applicationDeploy, err := applicationClient.Get(appName, metav1.GetOptions{})
	if err != nil {
		return errors.Errorf("Unable to get the application, err: %v", err)
	}
	for count := 0; count < int(experimentsDetails.ChaosDuration/2); count++ {

		if int(applicationDeploy.Status.AvailableReplicas) != experimentsDetails.Replicas {

			log.Infof("Application Pod Avaliable Count is: %s", strconv.Itoa(int(applicationDeploy.Status.AvailableReplicas)))
			applicationDeploy, err = applicationClient.Get(appName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to get the application, err: %v", err)
			}

			time.Sleep(2 * time.Second)
			//ChaosCurrentTimeStamp contains the current timestamp
			ChaosCurrentTimeStamp := time.Now().Unix()

			//ChaosDiffTimeStamp contains the difference of current timestamp and start timestamp
			//It will helpful to track the total chaos duration
			chaosDiffTimeStamp := ChaosCurrentTimeStamp - ChaosStartTimeStamp
			if int(chaosDiffTimeStamp) >= experimentsDetails.ChaosDuration {
				failFlag = true
				break
			}

		} else {
			break
		}
	}
	if failFlag == true {
		err = AutoscalerRecovery(experimentsDetails, clients, replicaCount, appName)
		if err != nil {
			return errors.Errorf("Unable to perform autoscaling, due to %v", err)
		}
		return errors.Errorf("Application pod fails to come in running state after Chaos Duration of %d sec", experimentsDetails.ChaosDuration)
	}
	// Keeping a wait time of 10s after all pod comes in running state
	// This is optional and used just for viewing the pod status
	time.Sleep(10 * time.Second)

	return nil
}

//AutoscalerRecovery scale back to initial number of replica
func AutoscalerRecovery(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, replicaCount int, appName string) error {

	applicationClient := clients.KubeClient.AppsV1().Deployments(experimentsDetails.ChaosNamespace)

	// Scale back to initial number of replicas
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		appUnderTest, err := applicationClient.Get(appName, metav1.GetOptions{})
		if err != nil {
			return errors.Errorf("Failed to get latest version of Application Deployment: %v", err)
		}

		appUnderTest.Spec.Replicas = int32Ptr(int32(replicaCount)) // modify replica count
		_, updateErr := applicationClient.Update(appUnderTest)
		return updateErr
	})
	if retryErr != nil {
		return errors.Errorf("Unable to scale the, due to: %v", retryErr)
	}
	log.Info("[Info]: Application pod started rolling back")

	applicationDeploy, err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS).Get(appName, metav1.GetOptions{})
	if err != nil {
		return errors.Errorf("Unable to get the application, err: %v", err)
	}

	failFlag := false
	// Check for 30 retries with 2secs of delay
	for count := 0; count < 30; count++ {

		if int(applicationDeploy.Status.AvailableReplicas) != replicaCount {

			applicationDeploy, err = applicationClient.Get(appName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to get the application, err: %v", err)
			}
			time.Sleep(2 * time.Second)
			if count == 30 {
				failFlag = true
				break
			}

		} else {
			break
		}
	}
	if failFlag == true {
		return errors.Errorf("Application fails to roll back")
	}
	log.Info("[RollBack]: Application Pod roll back to initial number of replicas")

	return nil
}

func int32Ptr(i int32) *int32 { return &i }
