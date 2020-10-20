package lib

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-autoscaler/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	retries "k8s.io/client-go/util/retry"

	"github.com/pkg/errors"
)

var err error

//PreparePodAutoscaler contains the prepration steps before chaos injection
func PreparePodAutoscaler(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	appName, replicaCount, err := GetApplicationDetails(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the replicaCount of the application, err: %v", err)
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	err = PodAutoscalerChaos(experimentsDetails, clients, replicaCount, appName, resultDetails, eventsDetails, chaosDetails)
	if err != nil {
		return errors.Errorf("Unable to perform autoscaling, err: %v", err)
	}

	err = AutoscalerRecovery(experimentsDetails, clients, replicaCount, appName)
	if err != nil {
		return errors.Errorf("Unable to recover the auto scaling, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//GetApplicationDetails is used to get the name and total number of replicas of the application
func GetApplicationDetails(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, int, error) {

	var appReplica int
	var appName string
	// Get Deployment replica count
	applicationList, err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS).List(metav1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(applicationList.Items) == 0 {
		return "", 0, errors.Errorf("Unable to find the application pods with matching labels, err: %v", err)
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
			return errors.Errorf("Failed to get latest version of Application Deployment, err: %v", err)
		}
		// modifying the replica count
		appUnderTest.Spec.Replicas = int32Ptr(replicas)
		_, updateErr := applicationClient.Update(appUnderTest)
		return updateErr
	})
	if retryErr != nil {
		return errors.Errorf("Unable to scale the application, err: %v", retryErr)
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

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	applicationClient := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS)
	isFailed := false

	err := retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			// signChan channel is used to transmit signal notifications.
			signChan := make(chan os.Signal, 1)
			// Catch and relay certain signal(s) to signChan channel.
			signal.Notify(signChan, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
		loop:
			for {
				endTime = time.After(timeDelay)
				select {
				case <-signChan:
					log.Info("[Chaos]: Killing process started because of terminated signal received")
					// updating the chaosresult after stopped
					failStep := "Pod Autoscaler experiment stopped!"
					types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
					result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

					// generating summary event in chaosengine
					msg := experimentsDetails.ExperimentName + " experiment has been aborted"
					types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
					events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

					// generating summary event in chaosresult
					types.SetResultEventAttributes(eventsDetails, types.StoppedVerdict, msg, "Warning", resultDetails)
					events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")

					if err = AutoscalerRecovery(experimentsDetails, clients, replicaCount, appName); err != nil {
						log.Errorf("Unable to perform autoscaling, err: %v", err)
					}
					os.Exit(1)
				case <-endTime:
					log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
					endTime = nil
					break loop
				}
			}
			applicationDeploy, err := applicationClient.Get(appName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to find the pod with name %v, err: %v", appName, err)
			}
			if int(applicationDeploy.Status.AvailableReplicas) != experimentsDetails.Replicas {
				log.Infof("Application Pod Available Count is: %v", applicationDeploy.Status.AvailableReplicas)
				isFailed = true

				return errors.Errorf("Application is not scaled yet, err: %v", err)
			}
			isFailed = false
			return nil

		})
	if isFailed {
		err = AutoscalerRecovery(experimentsDetails, clients, replicaCount, appName)
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

//AutoscalerRecovery scale back to initial number of replica
func AutoscalerRecovery(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, replicaCount int, appName string) error {

	applicationClient := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS)

	// Scale back to initial number of replicas
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		appUnderTest, err := applicationClient.Get(appName, metav1.GetOptions{})
		if err != nil {
			return errors.Errorf("Failed to find the latest version of Application Deployment with name %v, err: %v", appName, err)
		}

		appUnderTest.Spec.Replicas = int32Ptr(int32(replicaCount)) // modify replica count
		_, updateErr := applicationClient.Update(appUnderTest)
		return updateErr
	})
	if retryErr != nil {
		return errors.Errorf("Unable to scale the application pod, err: %v", retryErr)
	}
	log.Info("[Info]: Application pod started rolling back")

	err = retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			applicationDeploy, err := applicationClient.Get(appName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to find the pod with name %v, err: %v", appName, err)
			}
			if int(applicationDeploy.Status.AvailableReplicas) != experimentsDetails.Replicas {
				log.Infof("Application Pod Available Count is: %v", applicationDeploy.Status.AvailableReplicas)
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
