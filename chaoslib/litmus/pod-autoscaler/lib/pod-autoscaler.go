package lib

import (
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-autoscaler/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	retries "k8s.io/client-go/util/retry"
)

var (
	err                     error
	appsv1DeploymentClient  appsv1.DeploymentInterface
	appsv1StatefulsetClient appsv1.StatefulSetInterface
)

//PreparePodAutoscaler contains the preparation steps and chaos injection steps
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

		appsUnderTest, err := getDeploymentDetails(experimentsDetails)
		if err != nil {
			return stacktrace.Propagate(err, "could not get deployment details")
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
			return stacktrace.Propagate(err, "could not scale deployment")
		}

		if err = autoscalerRecoveryInDeployment(experimentsDetails, clients, appsUnderTest, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not revert scaling in deployment")
		}

	case "statefulset", "statefulsets":

		appsUnderTest, err := getStatefulsetDetails(experimentsDetails)
		if err != nil {
			return stacktrace.Propagate(err, "could not get statefulset details")
		}

		var stsList []string
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
			return stacktrace.Propagate(err, "could not scale statefulset")
		}

		if err = autoscalerRecoveryInStatefulset(experimentsDetails, clients, appsUnderTest, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not revert scaling in statefulset")
		}

	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{kind: %s}", experimentsDetails.AppKind), Reason: "application type is not supported"}
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

func getSliceOfTotalApplicationsTargeted(appList []experimentTypes.ApplicationUnderTest, experimentsDetails *experimentTypes.ExperimentDetails) []experimentTypes.ApplicationUnderTest {

	newAppListLength := math.Maximum(1, math.Adjustment(math.Minimum(experimentsDetails.AppAffectPercentage, 100), len(appList)))
	return appList[:newAppListLength]
}

//getDeploymentDetails is used to get the name and total number of replicas of the deployment
func getDeploymentDetails(experimentsDetails *experimentTypes.ExperimentDetails) ([]experimentTypes.ApplicationUnderTest, error) {

	deploymentList, err := appsv1DeploymentClient.List(context.Background(), metav1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{kind: deployment, labels: %s}", experimentsDetails.AppLabel), Reason: err.Error()}
	} else if len(deploymentList.Items) == 0 {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{kind: deployment, labels: %s}", experimentsDetails.AppLabel), Reason: "no deployment found with matching labels"}
	}
	var appsUnderTest []experimentTypes.ApplicationUnderTest
	for _, app := range deploymentList.Items {
		log.Infof("[Info]: Found deployment name '%s' with replica count '%d'", app.Name, int(*app.Spec.Replicas))
		appsUnderTest = append(appsUnderTest, experimentTypes.ApplicationUnderTest{AppName: app.Name, ReplicaCount: int(*app.Spec.Replicas)})
	}
	// Applying the APP_AFFECTED_PERC variable to determine the total target deployments to scale
	return getSliceOfTotalApplicationsTargeted(appsUnderTest, experimentsDetails), nil
}

//getStatefulsetDetails is used to get the name and total number of replicas of the statefulsets
func getStatefulsetDetails(experimentsDetails *experimentTypes.ExperimentDetails) ([]experimentTypes.ApplicationUnderTest, error) {

	statefulsetList, err := appsv1StatefulsetClient.List(context.Background(), metav1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{kind: statefulset, labels: %s}", experimentsDetails.AppLabel), Reason: err.Error()}
	} else if len(statefulsetList.Items) == 0 {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{kind: statefulset, labels: %s}", experimentsDetails.AppLabel), Reason: "no statefulset found with matching labels"}
	}

	appsUnderTest := []experimentTypes.ApplicationUnderTest{}
	for _, app := range statefulsetList.Items {
		log.Infof("[Info]: Found statefulset name '%s' with replica count '%d'", app.Name, int(*app.Spec.Replicas))
		appsUnderTest = append(appsUnderTest, experimentTypes.ApplicationUnderTest{AppName: app.Name, ReplicaCount: int(*app.Spec.Replicas)})
	}
	// Applying the APP_AFFECT_PERC variable to determine the total target deployments to scale
	return getSliceOfTotalApplicationsTargeted(appsUnderTest, experimentsDetails), nil
}

//podAutoscalerChaosInDeployment scales up the replicas of deployment and verify the status
func podAutoscalerChaosInDeployment(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appsUnderTest []experimentTypes.ApplicationUnderTest, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Scale Application
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		for _, app := range appsUnderTest {
			// Retrieve the latest version of Deployment before attempting update
			// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
			appUnderTest, err := appsv1DeploymentClient.Get(context.Background(), app.AppName, metav1.GetOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{kind: deployment, name: %s, namespace: %s}", app.AppName, experimentsDetails.AppNS), Reason: err.Error()}
			}
			// modifying the replica count
			appUnderTest.Spec.Replicas = int32Ptr(int32(experimentsDetails.Replicas))
			log.Infof("Updating deployment '%s' to number of replicas '%d'", appUnderTest.ObjectMeta.Name, experimentsDetails.Replicas)
			_, err = appsv1DeploymentClient.Update(context.Background(), appUnderTest, metav1.UpdateOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{kind: deployment, name: %s, namespace: %s}", app.AppName, experimentsDetails.AppNS), Reason: fmt.Sprintf("failed to scale deployment :%s", err.Error())}
			}
			common.SetTargets(app.AppName, "injected", "deployment", chaosDetails)
		}
		return nil
	})
	if retryErr != nil {
		return retryErr
	}
	log.Info("[Info]: The application started scaling")

	return deploymentStatusCheck(experimentsDetails, clients, appsUnderTest, resultDetails, eventsDetails, chaosDetails)
}

//podAutoscalerChaosInStatefulset scales up the replicas of statefulset and verify the status
func podAutoscalerChaosInStatefulset(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appsUnderTest []experimentTypes.ApplicationUnderTest, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Scale Application
	retryErr := retries.RetryOnConflict(retries.DefaultRetry, func() error {
		for _, app := range appsUnderTest {
			// Retrieve the latest version of Statefulset before attempting update
			// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
			appUnderTest, err := appsv1StatefulsetClient.Get(context.Background(), app.AppName, metav1.GetOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{kind: statefulset, name: %s, namespace: %s}", app.AppName, experimentsDetails.AppNS), Reason: err.Error()}
			}
			// modifying the replica count
			appUnderTest.Spec.Replicas = int32Ptr(int32(experimentsDetails.Replicas))
			_, err = appsv1StatefulsetClient.Update(context.Background(), appUnderTest, metav1.UpdateOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{kind: statefulset, name: %s, namespace: %s}", app.AppName, experimentsDetails.AppNS), Reason: fmt.Sprintf("failed to scale statefulset :%s", err.Error())}
			}
			common.SetTargets(app.AppName, "injected", "statefulset", chaosDetails)
		}
		return nil
	})
	if retryErr != nil {
		return retryErr
	}
	log.Info("[Info]: The application started scaling")

	return statefulsetStatusCheck(experimentsDetails, clients, appsUnderTest, resultDetails, eventsDetails, chaosDetails)
}

// deploymentStatusCheck check the status of deployment and verify the available replicas
func deploymentStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appsUnderTest []experimentTypes.ApplicationUnderTest, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()

	err = retry.
		Times(uint(experimentsDetails.ChaosDuration / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			for _, app := range appsUnderTest {
				deployment, err := appsv1DeploymentClient.Get(context.Background(), app.AppName, metav1.GetOptions{})
				if err != nil {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{kind: deployment, namespace: %s, name: %s}", experimentsDetails.AppNS, app.AppName), Reason: err.Error()}
				}
				if int(deployment.Status.ReadyReplicas) != experimentsDetails.Replicas {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{kind: deployment, namespace: %s, name: %s}", experimentsDetails.AppNS, app.AppName), Reason: fmt.Sprintf("failed to scale deployment, the desired replica count is: %v and ready replica count is: %v", experimentsDetails.Replicas, deployment.Status.ReadyReplicas)}
				}
			}
			return nil
		})

	if err != nil {
		if scaleErr := autoscalerRecoveryInDeployment(experimentsDetails, clients, appsUnderTest, chaosDetails); scaleErr != nil {
			return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(scaleErr).Error())}
		}
		return stacktrace.Propagate(err, "failed to scale replicas")
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

	err = retry.
		Times(uint(experimentsDetails.ChaosDuration / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			for _, app := range appsUnderTest {
				statefulset, err := appsv1StatefulsetClient.Get(context.Background(), app.AppName, metav1.GetOptions{})
				if err != nil {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{kind: statefulset, namespace: %s, name: %s}", experimentsDetails.AppNS, app.AppName), Reason: err.Error()}
				}
				if int(statefulset.Status.ReadyReplicas) != experimentsDetails.Replicas {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{kind: statefulset, namespace: %s, name: %s}", experimentsDetails.AppNS, app.AppName), Reason: fmt.Sprintf("failed to scale statefulset, the desired replica count is: %v and ready replica count is: %v", experimentsDetails.Replicas, statefulset.Status.ReadyReplicas)}
				}
			}
			return nil
		})

	if err != nil {
		if scaleErr := autoscalerRecoveryInStatefulset(experimentsDetails, clients, appsUnderTest, chaosDetails); scaleErr != nil {
			return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(scaleErr).Error())}
		}
		return stacktrace.Propagate(err, "failed to scale replicas")
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
			appUnderTest, err := appsv1DeploymentClient.Get(context.Background(), app.AppName, metav1.GetOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{kind: deployment, namespace: %s, name: %s}", experimentsDetails.AppNS, app.AppName), Reason: err.Error()}
			}
			appUnderTest.Spec.Replicas = int32Ptr(int32(app.ReplicaCount)) // modify replica count
			_, err = appsv1DeploymentClient.Update(context.Background(), appUnderTest, metav1.UpdateOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{kind: deployment, name: %s, namespace: %s}", app.AppName, experimentsDetails.AppNS), Reason: fmt.Sprintf("failed to revert scaling in deployment :%s", err.Error())}
			}
			common.SetTargets(app.AppName, "reverted", "deployment", chaosDetails)
		}
		return nil
	})

	if retryErr != nil {
		return retryErr
	}
	log.Info("[Info]: Application started rolling back to original replica count")

	return retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			for _, app := range appsUnderTest {
				applicationDeploy, err := appsv1DeploymentClient.Get(context.Background(), app.AppName, metav1.GetOptions{})
				if err != nil {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{kind: deployment, namespace: %s, name: %s}", experimentsDetails.AppNS, app.AppName), Reason: err.Error()}
				}
				if int(applicationDeploy.Status.ReadyReplicas) != app.ReplicaCount {
					log.Infof("[Info]: Application ready replica count is: %v", applicationDeploy.Status.ReadyReplicas)
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{kind: deployment, namespace: %s, name: %s}", experimentsDetails.AppNS, app.AppName), Reason: fmt.Sprintf("failed to rollback deployment scaling, the desired replica count is: %v and ready replica count is: %v", experimentsDetails.Replicas, applicationDeploy.Status.ReadyReplicas)}
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
			appUnderTest, err := appsv1StatefulsetClient.Get(context.Background(), app.AppName, metav1.GetOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{kind: statefulset, namespace: %s, name: %s}", experimentsDetails.AppNS, app.AppName), Reason: err.Error()}
			}

			appUnderTest.Spec.Replicas = int32Ptr(int32(app.ReplicaCount)) // modify replica count
			_, err = appsv1StatefulsetClient.Update(context.Background(), appUnderTest, metav1.UpdateOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{kind: statefulset, name: %s, namespace: %s}", app.AppName, experimentsDetails.AppNS), Reason: fmt.Sprintf("failed to revert scaling in statefulset :%s", err.Error())}
			}
			common.SetTargets(app.AppName, "reverted", "statefulset", chaosDetails)
		}
		return nil
	})
	if retryErr != nil {
		return retryErr
	}
	log.Info("[Info]: Application pod started rolling back")

	return retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			for _, app := range appsUnderTest {
				applicationDeploy, err := appsv1StatefulsetClient.Get(context.Background(), app.AppName, metav1.GetOptions{})
				if err != nil {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{kind: statefulset, namespace: %s, name: %s}", experimentsDetails.AppNS, app.AppName), Reason: err.Error()}
				}
				if int(applicationDeploy.Status.ReadyReplicas) != app.ReplicaCount {
					log.Infof("Application ready replica count is: %v", applicationDeploy.Status.ReadyReplicas)
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{kind: statefulset, namespace: %s, name: %s}", experimentsDetails.AppNS, app.AppName), Reason: fmt.Sprintf("failed to rollback statefulset scaling, the desired replica count is: %v and ready replica count is: %v", experimentsDetails.Replicas, applicationDeploy.Status.ReadyReplicas)}
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

	// waiting till the abort signal received
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
