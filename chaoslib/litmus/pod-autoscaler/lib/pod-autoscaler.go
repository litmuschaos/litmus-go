package lib

import (
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-autoscaler/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/sirupsen/logrus"
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

//PreparePodAutoscaler contains the prepration steps and chaos injection steps
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

		appsUnderTest, err := getDeploymentDetails(experimentsDetails, clients)
		if err != nil {
			return errors.Errorf("fail to get the name & initial replica count of the deployment, err: %v", err)
		}

		deploymentList := []string{}
		for _, deployment := range appsUnderTest {
			deploymentList = append(deploymentList, deployment.AppName)
		}
		log.InfoWithValues("[Info]: Details of Deployments under chaos injection", logrus.Fields{
			"Number Of Deployment": len(deploymentList),
			"Target Deployments":   deploymentList,
		})

		//calling go routine which will continuously watch for the abort signal
		go abortPodAutoScalerChaos(appsUnderTest, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails)

		if err = podAutoscalerChaosInDeployment(experimentsDetails, clients, appsUnderTest, resultDetails, eventsDetails, chaosDetails); err != nil {
			return errors.Errorf("fail to perform autoscaling, err: %v", err)
		}

		if err = autoscalerRecoveryInDeployment(experimentsDetails, clients, appsUnderTest, chaosDetails); err != nil {
			return errors.Errorf("fail to rollback the autoscaling, err: %v", err)
		}

	case "statefulset", "statefulsets":

		appsUnderTest, err := getStatefulsetDetails(experimentsDetails, clients)
		if err != nil {
			return errors.Errorf("fail to get the name & initial replica count of the statefulset, err: %v", err)
		}

		stsList := []string{}
		for _, sts := range appsUnderTest {
			stsList = append(stsList, sts.AppName)
		}
		log.InfoWithValues("[Info]: Details of Statefulsets under chaos injection", logrus.Fields{
			"Number Of Statefulsets": len(stsList),
			"Target Statefulsets":    stsList,
		})

		//calling go routine which will continuously watch for the abort signal
		go abortPodAutoScalerChaos(appsUnderTest, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails)

		if err = podAutoscalerChaosInStatefulset(experimentsDetails, clients, appsUnderTest, resultDetails, eventsDetails, chaosDetails); err != nil {
			return errors.Errorf("fail to perform autoscaling, err: %v", err)
		}

		if err = autoscalerRecoveryInStatefulset(experimentsDetails, clients, appsUnderTest, chaosDetails); err != nil {
			return errors.Errorf("fail to rollback the autoscaling, err: %v", err)
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

func getSliceOfTotalApplicationsTargeted(appList []experimentTypes.ApplicationUnderTest, experimentsDetails *experimentTypes.ExperimentDetails) ([]experimentTypes.ApplicationUnderTest, error) {

	slice := int(math.Round(float64(len(appList)*experimentsDetails.AppAffectPercentage) / float64(100)))
	if slice < 0 || slice > len(appList) {
		return nil, errors.Errorf("slice of applications to target out of range %d/%d", slice, len(appList))
	}
	return appList[:slice], nil
}

//getDeploymentDetails is used to get the name and total number of replicas of the deployment
func getDeploymentDetails(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) ([]experimentTypes.ApplicationUnderTest, error) {

	deploymentList, err := appsv1DeploymentClient.List(metav1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(deploymentList.Items) == 0 {
		return nil, errors.Errorf("fail to get the deployments with matching labels, err: %v", err)
	}
	appsUnderTest := []experimentTypes.ApplicationUnderTest{}
	for _, app := range deploymentList.Items {
		log.Infof("[Info]: Found deployment name '%s' with replica count '%d'", app.Name, int(*app.Spec.Replicas))
		appsUnderTest = append(appsUnderTest, experimentTypes.ApplicationUnderTest{AppName: app.Name, ReplicaCount: int(*app.Spec.Replicas)})
	}
	// Applying the APP_AFFECT_PERC variable to determine the total target deployments to scale
	return getSliceOfTotalApplicationsTargeted(appsUnderTest, experimentsDetails)

}

//getStatefulsetDetails is used to get the name and total number of replicas of the statefulsets
func getStatefulsetDetails(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) ([]experimentTypes.ApplicationUnderTest, error) {

	statefulsetList, err := appsv1StatefulsetClient.List(metav1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(statefulsetList.Items) == 0 {
		return nil, errors.Errorf("fail to get the statefulsets with matching labels, err: %v", err)
	}

	appsUnderTest := []experimentTypes.ApplicationUnderTest{}
	for _, app := range statefulsetList.Items {
		log.Infof("[Info]: Found statefulset name '%s' with replica count '%d'", app.Name, int(*app.Spec.Replicas))
		appsUnderTest = append(appsUnderTest, experimentTypes.ApplicationUnderTest{AppName: app.Name, ReplicaCount: int(*app.Spec.Replicas)})
	}
	// Applying the APP_AFFECT_PERC variable to determine the total target deployments to scale
	return getSliceOfTotalApplicationsTargeted(appsUnderTest, experimentsDetails)
}

//podAutoscalerChaosInDeployment scales up the replicas of deployment and verify the status
func podAutoscalerChaosInDeployment(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appsUnderTest []experimentTypes.ApplicationUnderTest, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Scale Application
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		for _, app := range appsUnderTest {
			// Retrieve the latest version of Deployment before attempting update
			// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
			appUnderTest, err := appsv1DeploymentClient.Get(app.AppName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("fail to get latest version of application deployment, err: %v", err)
			}
			// modifying the replica count
			appUnderTest.Spec.Replicas = int32Ptr(int32(experimentsDetails.Replicas))
			log.Infof("Updating deployment '%s' to number of replicas '%d'", appUnderTest.ObjectMeta.Name, experimentsDetails.Replicas)
			_, err = appsv1DeploymentClient.Update(appUnderTest)
			if err != nil {
				return err
			}
			common.SetTargets(app.AppName, "injected", "deployment", chaosDetails)
		}
		return nil
	})
	if retryErr != nil {
		return errors.Errorf("fail to update the replica count of the deployment, err: %v", retryErr)
	}
	log.Info("[Info]: The application started scaling")

	if err = deploymentStatusCheck(experimentsDetails, clients, appsUnderTest, resultDetails, eventsDetails, chaosDetails); err != nil {
		return errors.Errorf("application deployment status check failed, err: %v", err)
	}

	return nil
}

//podAutoscalerChaosInStatefulset scales up the replicas of statefulset and verify the status
func podAutoscalerChaosInStatefulset(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appsUnderTest []experimentTypes.ApplicationUnderTest, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Scale Application
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		for _, app := range appsUnderTest {
			// Retrieve the latest version of Statefulset before attempting update
			// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
			appUnderTest, err := appsv1StatefulsetClient.Get(app.AppName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("fail to get latest version of the target statefulset application , err: %v", err)
			}
			// modifying the replica count
			appUnderTest.Spec.Replicas = int32Ptr(int32(experimentsDetails.Replicas))
			_, err = appsv1StatefulsetClient.Update(appUnderTest)
			if err != nil {
				return err
			}
			common.SetTargets(app.AppName, "injected", "statefulset", chaosDetails)
		}
		return nil
	})
	if retryErr != nil {
		return errors.Errorf("fail to update the replica count of the statefulset application, err: %v", retryErr)
	}
	log.Info("[Info]: The application started scaling")

	if err = statefulsetStatusCheck(experimentsDetails, clients, appsUnderTest, resultDetails, eventsDetails, chaosDetails); err != nil {
		return errors.Errorf("statefulset application status check failed, err: %v", err)
	}

	return nil
}

// deploymentStatusCheck check the status of deployment and verify the available replicas
func deploymentStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appsUnderTest []experimentTypes.ApplicationUnderTest, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	isFailed := false

	err = retry.
		Times(uint(experimentsDetails.ChaosDuration / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			for _, app := range appsUnderTest {
				deployment, err := appsv1DeploymentClient.Get(app.AppName, metav1.GetOptions{})
				if err != nil {
					return errors.Errorf("fail to find the deployment with name %v, err: %v", app.AppName, err)
				}
				if int(deployment.Status.ReadyReplicas) != experimentsDetails.Replicas {
					isFailed = true
					return errors.Errorf("application %s is not scaled yet, the desired replica count is: %v and ready replica count is: %v", app.AppName, experimentsDetails.Replicas, deployment.Status.ReadyReplicas)
				}
			}
			isFailed = false
			return nil
		})

	if isFailed {
		if err = autoscalerRecoveryInDeployment(experimentsDetails, clients, appsUnderTest, chaosDetails); err != nil {
			return errors.Errorf("fail to perform the autoscaler recovery of the deployment, err: %v", err)
		}
		return errors.Errorf("fail to scale the deployment to the desired replica count in the given chaos duration")
	}
	if err != nil {
		return err
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())
	if duration < experimentsDetails.ChaosDuration {
		log.Info("[Wait]: Waiting for completion of chaos duration")
		time.Sleep(time.Duration(experimentsDetails.ChaosDuration-duration) * time.Second)
	}

	return nil
}

// statefulsetStatusCheck check the status of statefulset and verify the available replicas
func statefulsetStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appsUnderTest []experimentTypes.ApplicationUnderTest, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	isFailed := false

	err = retry.
		Times(uint(experimentsDetails.ChaosDuration / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			for _, app := range appsUnderTest {
				statefulset, err := appsv1StatefulsetClient.Get(app.AppName, metav1.GetOptions{})
				if err != nil {
					return errors.Errorf("fail to find the statefulset with name %v, err: %v", app.AppName, err)
				}
				if int(statefulset.Status.ReadyReplicas) != experimentsDetails.Replicas {
					isFailed = true
					return errors.Errorf("application %s is not scaled yet, the desired replica count is: %v and ready replica count is: %v", app.AppName, experimentsDetails.Replicas, statefulset.Status.ReadyReplicas)
				}
			}
			isFailed = false
			return nil
		})

	if isFailed {
		if err = autoscalerRecoveryInStatefulset(experimentsDetails, clients, appsUnderTest, chaosDetails); err != nil {
			return errors.Errorf("fail to perform the autoscaler recovery of the application, err: %v", err)
		}
		return errors.Errorf("fail to scale the application to the desired replica count in the given chaos duration")
	}
	if err != nil {
		return err
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	duration := int(time.Since(ChaosStartTimeStamp).Seconds())
	if duration < experimentsDetails.ChaosDuration {
		log.Info("[Wait]: Waiting for completion of chaos duration")
		time.Sleep(time.Duration(experimentsDetails.ChaosDuration-duration) * time.Second)
	}

	return nil
}

//autoscalerRecoveryInDeployment rollback the replicas to initial values in deployment
func autoscalerRecoveryInDeployment(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appsUnderTest []experimentTypes.ApplicationUnderTest, chaosDetails *types.ChaosDetails) error {

	// Scale back to initial number of replicas
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		for _, app := range appsUnderTest {
			appUnderTest, err := appsv1DeploymentClient.Get(app.AppName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("fail to find the latest version of Application Deployment with name %v, err: %v", app.AppName, err)
			}

			appUnderTest.Spec.Replicas = int32Ptr(int32(app.ReplicaCount)) // modify replica count
			_, err = appsv1DeploymentClient.Update(appUnderTest)
			if err != nil {
				return err
			}
			common.SetTargets(app.AppName, "reverted", "deployment", chaosDetails)
		}
		return nil
	})
	if retryErr != nil {
		return errors.Errorf("fail to rollback the deployment, err: %v", retryErr)
	}
	log.Info("[Info]: Application started rolling back to original replica count")

	return retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			for _, app := range appsUnderTest {
				applicationDeploy, err := appsv1DeploymentClient.Get(app.AppName, metav1.GetOptions{})
				if err != nil {
					return errors.Errorf("fail to find the deployment with name %v, err: %v", app.AppName, err)
				}
				if int(applicationDeploy.Status.ReadyReplicas) != app.ReplicaCount {
					log.Infof("[Info]: Application ready replica count is: %v", applicationDeploy.Status.ReadyReplicas)
					return errors.Errorf("fail to rollback to original replica count, err: %v", err)
				}
			}
			log.Info("[RollBack]: Application rollback to the initial number of replicas")
			return nil
		})
}

//autoscalerRecoveryInStatefulset rollback the replicas to initial values in deployment
func autoscalerRecoveryInStatefulset(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appsUnderTest []experimentTypes.ApplicationUnderTest, chaosDetails *types.ChaosDetails) error {

	// Scale back to initial number of replicas
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		for _, app := range appsUnderTest {
			// Retrieve the latest version of Statefulset before attempting update
			// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
			appUnderTest, err := appsv1StatefulsetClient.Get(app.AppName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("failed to find the latest version of Statefulset with name %v, err: %v", app.AppName, err)
			}

			appUnderTest.Spec.Replicas = int32Ptr(int32(app.ReplicaCount)) // modify replica count
			_, err = appsv1StatefulsetClient.Update(appUnderTest)
			if err != nil {
				return err
			}
			common.SetTargets(app.AppName, "reverted", "statefulset", chaosDetails)
		}
		return nil
	})
	if retryErr != nil {
		return errors.Errorf("fail to rollback the statefulset, err: %v", retryErr)
	}
	log.Info("[Info]: Application pod started rolling back")

	return retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			for _, app := range appsUnderTest {
				applicationDeploy, err := appsv1StatefulsetClient.Get(app.AppName, metav1.GetOptions{})
				if err != nil {
					return errors.Errorf("fail to get the statefulset with name %v, err: %v", app.AppName, err)
				}
				if int(applicationDeploy.Status.ReadyReplicas) != app.ReplicaCount {
					log.Infof("Application ready replica count is: %v", applicationDeploy.Status.ReadyReplicas)
					return errors.Errorf("fail to roll back to original replica count, err: %v", err)
				}
			}
			log.Info("[RollBack]: Application roll back to initial number of replicas")
			return nil
		})
}

func int32Ptr(i int32) *int32 { return &i }

//abortPodAutoScalerChaos go routine will continuously watch for the abort signal for the entire chaos duration and generate the required events and result
func abortPodAutoScalerChaos(appsUnderTest []experimentTypes.ApplicationUnderTest, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) {

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)

	// waiting till the abort signal recieved
	<-signChan

	log.Info("[Chaos]: Revert Started")
	// Note that we are attempting recovery (in this case scaling down to original replica count) after ..
	// .. the tasks to patch results & generate events. This is so because the func AutoscalerRecovery..
	// ..takes more time to complete - it involves a status check post the downscale. We have a period of ..
	// .. few seconds before the pod deletion/removal occurs from the time the TERM is caught and thereby..
	// ..run the risk of not updating the status of the objects/create events. With the current approach..
	// ..tests indicate we succeed with the downscale/patch call, even if the status checks take longer
	// As such, this is a workaround, and other solutions such as usage of pre-stop hooks etc., need to be explored
	// Other experiments have simpler "recoveries" that are more or less guaranteed to work.
	switch strings.ToLower(experimentsDetails.AppKind) {
	case "deployment", "deployments":
		if err := autoscalerRecoveryInDeployment(experimentsDetails, clients, appsUnderTest, chaosDetails); err != nil {
			log.Errorf("the recovery after abortion failed err: %v", err)
		}

	case "statefulset", "statefulsets":
		if err := autoscalerRecoveryInStatefulset(experimentsDetails, clients, appsUnderTest, chaosDetails); err != nil {
			log.Errorf("the recovery after abortion failed err: %v", err)
		}

	default:
		log.Errorf("application type '%s' is not supported for the chaos", experimentsDetails.AppKind)
	}
	log.Info("[Chaos]: Revert Completed")

	os.Exit(1)
}
