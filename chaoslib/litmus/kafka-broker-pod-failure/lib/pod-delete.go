package lib

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/workloads"
	"github.com/palantir/stacktrace"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PreparePodDelete contains the prepration steps before chaos injection
func PreparePodDelete(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.ChaoslibDetail.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.ChaoslibDetail.RampTime)
		common.WaitForDuration(experimentsDetails.ChaoslibDetail.RampTime)
	}

	switch strings.ToLower(experimentsDetails.ChaoslibDetail.Sequence) {
	case "serial":
		if err := injectChaosInSerialMode(experimentsDetails, clients, chaosDetails, eventsDetails, resultDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err := injectChaosInParallelMode(experimentsDetails, clients, chaosDetails, eventsDetails, resultDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.ChaoslibDetail.Sequence)}
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.ChaoslibDetail.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.ChaoslibDetail.RampTime)
		common.WaitForDuration(experimentsDetails.ChaoslibDetail.RampTime)
	}
	return nil
}

// injectChaosInSerialMode delete the kafka broker pods in serial mode(one by one)
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails, resultDetails *types.ResultDetails) error {

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	GracePeriod := int64(0)
	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaoslibDetail.ChaosDuration {
		// Get the target pod details for the chaos execution
		// if the target pod is not defined it will derive the random target pod list using pod affected percentage
		if experimentsDetails.KafkaBroker == "" && chaosDetails.AppDetail == nil {
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "please provide one of the appLabel or KAFKA_BROKER"}
		}

		podsAffectedPerc, _ := strconv.Atoi(experimentsDetails.ChaoslibDetail.PodsAffectedPerc)
		targetPodList, err := common.GetPodList(experimentsDetails.KafkaBroker, podsAffectedPerc, clients, chaosDetails)
		if err != nil {
			return err
		}

		// deriving the parent name of the target resources
		for _, pod := range targetPodList.Items {
			kind, parentName, err := workloads.GetPodOwnerTypeAndName(&pod, clients.DynamicClient)
			if err != nil {
				return err
			}
			common.SetParentName(parentName, kind, pod.Namespace, chaosDetails)
		}
		for _, target := range chaosDetails.ParentsResources {
			common.SetTargets(target.Name, "targeted", target.Kind, chaosDetails)
		}

		if experimentsDetails.ChaoslibDetail.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		//Deleting the application pod
		for _, pod := range targetPodList.Items {

			log.InfoWithValues("[Info]: Killing the following pods", logrus.Fields{
				"PodName": pod.Name})

			if experimentsDetails.ChaoslibDetail.Force {
				err = clients.KubeClient.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, v1.DeleteOptions{GracePeriodSeconds: &GracePeriod})
			} else {
				err = clients.KubeClient.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, v1.DeleteOptions{})
			}
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to delete the target pod: %s", err.Error())}
			}

			switch chaosDetails.Randomness {
			case true:
				if err := common.RandomInterval(experimentsDetails.ChaoslibDetail.ChaosInterval); err != nil {
					return stacktrace.Propagate(err, "could not get random chaos interval")
				}
			default:
				//Waiting for the chaos interval after chaos injection
				if experimentsDetails.ChaoslibDetail.ChaosInterval != "" {
					log.Infof("[Wait]: Wait for the chaos interval %vs", experimentsDetails.ChaoslibDetail.ChaosInterval)
					waitTime, _ := strconv.Atoi(experimentsDetails.ChaoslibDetail.ChaosInterval)
					common.WaitForDuration(waitTime)
				}
			}

			//Verify the status of pod after the chaos injection
			log.Info("[Status]: Verification for the recreation of application pod")
			for _, parent := range chaosDetails.ParentsResources {
				target := types.AppDetails{
					Names:     []string{parent.Name},
					Kind:      parent.Kind,
					Namespace: parent.Namespace,
				}
				if err = status.CheckUnTerminatedPodStatusesByWorkloadName(target, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients); err != nil {
					return stacktrace.Propagate(err, "could not check pod statuses by workload names")
				}
			}
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	log.Infof("[Completion]: %v chaos is done", experimentsDetails.ExperimentName)

	return nil
}

// injectChaosInParallelMode delete the kafka broker pods in parallel mode (all at once)
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails, resultDetails *types.ResultDetails) error {

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	GracePeriod := int64(0)
	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaoslibDetail.ChaosDuration {
		// Get the target pod details for the chaos execution
		// if the target pod is not defined it will derive the random target pod list using pod affected percentage
		if experimentsDetails.KafkaBroker == "" && chaosDetails.AppDetail == nil {
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "please provide one of the appLabel or KAFKA_BROKER"}
		}
		podsAffectedPerc, _ := strconv.Atoi(experimentsDetails.ChaoslibDetail.PodsAffectedPerc)
		targetPodList, err := common.GetPodList(experimentsDetails.KafkaBroker, podsAffectedPerc, clients, chaosDetails)
		if err != nil {
			return stacktrace.Propagate(err, "could not get target pods")
		}

		// deriving the parent name of the target resources
		for _, pod := range targetPodList.Items {
			kind, parentName, err := workloads.GetPodOwnerTypeAndName(&pod, clients.DynamicClient)
			if err != nil {
				return stacktrace.Propagate(err, "could not get pod owner name and kind")
			}
			common.SetParentName(parentName, kind, pod.Namespace, chaosDetails)
		}
		for _, target := range chaosDetails.ParentsResources {
			common.SetTargets(target.Name, "targeted", target.Kind, chaosDetails)
		}

		if experimentsDetails.ChaoslibDetail.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		//Deleting the application pod
		for _, pod := range targetPodList.Items {

			log.InfoWithValues("[Info]: Killing the following pods", logrus.Fields{
				"PodName": pod.Name})

			if experimentsDetails.ChaoslibDetail.Force {
				err = clients.KubeClient.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, v1.DeleteOptions{GracePeriodSeconds: &GracePeriod})
			} else {
				err = clients.KubeClient.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, v1.DeleteOptions{})
			}
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to delete the target pod: %s", err.Error())}
			}
		}

		switch chaosDetails.Randomness {
		case true:
			if err := common.RandomInterval(experimentsDetails.ChaoslibDetail.ChaosInterval); err != nil {
				return stacktrace.Propagate(err, "could not get random chaos interval")
			}
		default:
			//Waiting for the chaos interval after chaos injection
			if experimentsDetails.ChaoslibDetail.ChaosInterval != "" {
				log.Infof("[Wait]: Wait for the chaos interval %vs", experimentsDetails.ChaoslibDetail.ChaosInterval)
				waitTime, _ := strconv.Atoi(experimentsDetails.ChaoslibDetail.ChaosInterval)
				common.WaitForDuration(waitTime)
			}
		}

		//Verify the status of pod after the chaos injection
		log.Info("[Status]: Verification for the recreation of application pod")
		for _, parent := range chaosDetails.ParentsResources {
			target := types.AppDetails{
				Names:     []string{parent.Name},
				Kind:      parent.Kind,
				Namespace: parent.Namespace,
			}
			if err = status.CheckUnTerminatedPodStatusesByWorkloadName(target, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients); err != nil {
				return stacktrace.Propagate(err, "could not check pod statuses by workload names")
			}
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	log.Infof("[Completion]: %v chaos is done", experimentsDetails.ExperimentName)

	return nil
}
