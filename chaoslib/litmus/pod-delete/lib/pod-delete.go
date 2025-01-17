package lib

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/workloads"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PreparePodDelete contains the preparation steps before chaos injection
func PreparePodDelete(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PreparePodDeleteFault")
	defer span.End()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//set up the tunables if provided in range
	SetChaosTunables(experimentsDetails)

	log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
		"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
		"Sequence":         experimentsDetails.Sequence,
	})

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err := injectChaosInSerialMode(ctx, experimentsDetails, clients, chaosDetails, eventsDetails, resultDetails); err != nil {
			span.SetStatus(codes.Error, "could not run chaos in serial mode")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err := injectChaosInParallelMode(ctx, experimentsDetails, clients, chaosDetails, eventsDetails, resultDetails); err != nil {
			span.SetStatus(codes.Error, "could not run chaos in parallel mode")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		errReason := fmt.Sprintf("sequence '%s' is not supported", experimentsDetails.Sequence)
		span.SetStatus(codes.Error, errReason)
		err := cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: errReason}
		span.RecordError(err)
		return err
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// injectChaosInSerialMode delete the target application pods serial mode(one by one)
func injectChaosInSerialMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails, resultDetails *types.ResultDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectPodDeleteFaultInSerialMode")
	defer span.End()

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			span.SetStatus(codes.Error, "could not run the probes during chaos")
			span.RecordError(err)
			return err
		}
	}

	GracePeriod := int64(0)
	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {
		// Get the target pod details for the chaos execution
		// if the target pod is not defined it will derive the random target pod list using pod affected percentage
		if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail == nil {
			span.SetStatus(codes.Error, "provide one of the appLabel or TARGET_PODS")
			err := cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "provide one of the appLabel or TARGET_PODS"}
			span.RecordError(err)
			return err
		}

		targetPodList, err := common.GetTargetPods(experimentsDetails.NodeLabel, experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
		if err != nil {
			span.SetStatus(codes.Error, "could not get target pods")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not get target pods")
		}

		// deriving the parent name of the target resources
		for _, pod := range targetPodList.Items {
			kind, parentName, err := workloads.GetPodOwnerTypeAndName(&pod, clients.DynamicClient)
			if err != nil {
				span.SetStatus(codes.Error, "could not get pod owner name and kind")
				span.RecordError(err)
				return stacktrace.Propagate(err, "could not get pod owner name and kind")
			}
			common.SetParentName(parentName, kind, pod.Namespace, chaosDetails)
		}
		for _, target := range chaosDetails.ParentsResources {
			common.SetTargets(target.Name, "targeted", target.Kind, chaosDetails)
		}

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		//Deleting the application pod
		for _, pod := range targetPodList.Items {

			log.InfoWithValues("[Info]: Killing the following pods", logrus.Fields{
				"PodName": pod.Name})

			if experimentsDetails.Force {
				err = clients.KubeClient.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, v1.DeleteOptions{GracePeriodSeconds: &GracePeriod})
			} else {
				err = clients.KubeClient.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, v1.DeleteOptions{})
			}
			if err != nil {
				span.SetStatus(codes.Error, "could not delete the target pod")
				span.RecordError(err)
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to delete the target pod: %s", err.Error())}
			}

			switch chaosDetails.Randomness {
			case true:
				if err := common.RandomInterval(experimentsDetails.ChaosInterval); err != nil {
					span.SetStatus(codes.Error, "could not get random chaos interval")
					span.RecordError(err)
					return stacktrace.Propagate(err, "could not get random chaos interval")
				}
			default:
				//Waiting for the chaos interval after chaos injection
				if experimentsDetails.ChaosInterval != "" {
					log.Infof("[Wait]: Wait for the chaos interval %vs", experimentsDetails.ChaosInterval)
					waitTime, _ := strconv.Atoi(experimentsDetails.ChaosInterval)
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
				if err = status.CheckUnTerminatedPodStatusesByWorkloadName(target, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
					span.SetStatus(codes.Error, "could not check pod statuses by workload names")
					span.RecordError(err)
					return stacktrace.Propagate(err, "could not check pod statuses by workload names")
				}
			}

			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}

	}
	log.Infof("[Completion]: %v chaos is done", experimentsDetails.ExperimentName)

	return nil

}

// injectChaosInParallelMode delete the target application pods in parallel mode (all at once)
func injectChaosInParallelMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails, resultDetails *types.ResultDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectPodDeleteFaultInParallelMode")
	defer span.End()

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	GracePeriod := int64(0)
	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {
		// Get the target pod details for the chaos execution
		// if the target pod is not defined it will derive the random target pod list using pod affected percentage
		if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail == nil {
			span.SetStatus(codes.Error, "please provide one of the appLabel or TARGET_PODS")
			err := cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "please provide one of the appLabel or TARGET_PODS"}
			span.RecordError(err)
			return err
		}
		targetPodList, err := common.GetTargetPods(experimentsDetails.NodeLabel, experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
		if err != nil {
			span.SetStatus(codes.Error, "could not get target pods")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not get target pods")
		}

		// deriving the parent name of the target resources
		for _, pod := range targetPodList.Items {
			kind, parentName, err := workloads.GetPodOwnerTypeAndName(&pod, clients.DynamicClient)
			if err != nil {
				span.SetStatus(codes.Error, "could not get pod owner name and kind")
				span.RecordError(err)
				return stacktrace.Propagate(err, "could not get pod owner name and kind")
			}
			common.SetParentName(parentName, kind, pod.Namespace, chaosDetails)
		}
		for _, target := range chaosDetails.ParentsResources {
			common.SetTargets(target.Name, "targeted", target.Kind, chaosDetails)
		}

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		//Deleting the application pod
		for _, pod := range targetPodList.Items {

			log.InfoWithValues("[Info]: Killing the following pods", logrus.Fields{
				"PodName": pod.Name})

			if experimentsDetails.Force {
				err = clients.KubeClient.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, v1.DeleteOptions{GracePeriodSeconds: &GracePeriod})
			} else {
				err = clients.KubeClient.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, v1.DeleteOptions{})
			}
			if err != nil {
				span.SetStatus(codes.Error, "could not delete the target pod")
				span.RecordError(err)
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to delete the target pod: %s", err.Error())}
			}
		}

		switch chaosDetails.Randomness {
		case true:
			if err := common.RandomInterval(experimentsDetails.ChaosInterval); err != nil {
				span.SetStatus(codes.Error, "could not get random chaos interval")
				return stacktrace.Propagate(err, "could not get random chaos interval")
			}
		default:
			//Waiting for the chaos interval after chaos injection
			if experimentsDetails.ChaosInterval != "" {
				log.Infof("[Wait]: Wait for the chaos interval %vs", experimentsDetails.ChaosInterval)
				waitTime, _ := strconv.Atoi(experimentsDetails.ChaosInterval)
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
			if err = status.CheckUnTerminatedPodStatusesByWorkloadName(target, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
				span.SetStatus(codes.Error, "could not check pod statuses by workload names")
				span.RecordError(err)
				return stacktrace.Propagate(err, "could not check pod statuses by workload names")
			}
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	log.Infof("[Completion]: %v chaos is done", experimentsDetails.ExperimentName)

	return nil
}

// SetChaosTunables will setup a random value within a given range of values
// If the value is not provided in range it'll setup the initial provided value.
func SetChaosTunables(experimentsDetails *experimentTypes.ExperimentDetails) {
	experimentsDetails.PodsAffectedPerc = common.ValidateRange(experimentsDetails.PodsAffectedPerc)
	experimentsDetails.Sequence = common.GetRandomSequence(experimentsDetails.Sequence)
}
