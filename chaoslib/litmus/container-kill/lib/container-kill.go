package lib

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/palantir/stacktrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrepareContainerKill contains the preparation steps before chaos injection
func PrepareContainerKill(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PrepareContainerKillFault")
	defer span.End()

	var err error
	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail == nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "provide one of the appLabel or TARGET_PODS"}
	}
	//Set up the tunables if provided in range
	SetChaosTunables(experimentsDetails)

	log.InfoWithValues("[Info]: The tunables are:", logrus.Fields{
		"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
		"Sequence":         experimentsDetails.Sequence,
	})

	targetPodList, err := common.GetTargetPods(experimentsDetails.NodeLabel, experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
	if err != nil {
		span.SetStatus(codes.Error, "Unable to get the target pods")
		span.RecordError(err)
		return stacktrace.Propagate(err, "could not get target pods")
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// Getting the serviceAccountName, need permission inside helper pod to create the events
	if experimentsDetails.ChaosServiceAccount == "" {
		experimentsDetails.ChaosServiceAccount, err = common.GetServiceAccount(experimentsDetails.ChaosNamespace, experimentsDetails.ChaosPodName, clients)
		if err != nil {
			span.SetStatus(codes.Error, "Unable to get the experiment service account")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not  experiment service account")
		}
	}

	if experimentsDetails.EngineName != "" {
		if err := common.SetHelperData(chaosDetails, experimentsDetails.SetHelperData, clients); err != nil {
			span.SetStatus(codes.Error, "Unable to set helper data")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not set helper data")
		}
	}

	experimentsDetails.IsTargetContainerProvided = experimentsDetails.TargetContainer != ""
	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(ctx, experimentsDetails, targetPodList, clients, chaosDetails, resultDetails, eventsDetails); err != nil {
			span.SetStatus(codes.Error, "Unable to run chaos in serial mode")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err = injectChaosInParallelMode(ctx, experimentsDetails, targetPodList, clients, chaosDetails, resultDetails, eventsDetails); err != nil {
			span.SetStatus(codes.Error, "Unable to run chaos in parallel mode")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		span.SetStatus(codes.Error, "Sequence not supported")
		err := cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
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

// injectChaosInSerialMode kill the container of all target application serially (one by one)
func injectChaosInSerialMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectContainerKillFaultInSerialMode")
	defer span.End()
	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			span.SetStatus(codes.Error, "failed to run probes")
			span.RecordError(err)
			return err
		}
	}

	// creating the helper pod to perform container kill chaos
	for _, pod := range targetPodList.Items {

		//Get the target container name of the application pod
		if !experimentsDetails.IsTargetContainerProvided {
			experimentsDetails.TargetContainer = pod.Spec.Containers[0].Name
		}

		runID := stringutils.GetRunID()

		if err := createHelperPod(ctx, experimentsDetails, clients, chaosDetails, fmt.Sprintf("%s:%s:%s", pod.Name, pod.Namespace, experimentsDetails.TargetContainer), pod.Spec.NodeName, runID); err != nil {
			span.SetStatus(codes.Error, "failed to create helper pod")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not create helper pod")
		}

		appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, runID)

		//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
		log.Info("[Status]: Checking the status of the helper pods")
		if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			span.SetStatus(codes.Error, "failed to check helper status")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not check helper status")
		}

		// Wait till the completion of the helper pod
		// set an upper limit for the waiting time
		log.Info("[Wait]: waiting till the completion of the helper pod")
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, common.GetContainerNames(chaosDetails)...)
		if err != nil || podStatus == "Failed" {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			err := common.HelperFailedError(err, appLabel, experimentsDetails.ChaosNamespace, true)
			span.SetStatus(codes.Error, "failed to wait for completion of helper pod")
			span.RecordError(err)
			return err
		}

		//Deleting all the helper pod for container-kill chaos
		log.Info("[Cleanup]: Deleting all the helper pods")
		if err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			span.SetStatus(codes.Error, "failed to delete helper pod(s)")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not delete helper pod(s)")
		}
	}
	return nil
}

// injectChaosInParallelMode kill the container of all target application in parallel mode (all at once)
func injectChaosInParallelMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectContainerKillFaultInParallelMode")
	defer span.End()
	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	runID := stringutils.GetRunID()
	targets := common.FilterPodsForNodes(targetPodList, experimentsDetails.TargetContainer)

	for node, tar := range targets {
		var targetsPerNode []string
		for _, k := range tar.Target {
			targetsPerNode = append(targetsPerNode, fmt.Sprintf("%s:%s:%s", k.Name, k.Namespace, k.TargetContainer))
		}

		if err := createHelperPod(ctx, experimentsDetails, clients, chaosDetails, strings.Join(targetsPerNode, ";"), node, runID); err != nil {
			span.SetStatus(codes.Error, "failed to create helper pod")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not create helper pod")
		}
	}

	appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, runID)

	//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pods")
	if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		span.SetStatus(codes.Error, "failed to check helper status")
		span.RecordError(err)
		return stacktrace.Propagate(err, "could not check helper status")
	}

	// Wait till the completion of the helper pod
	// set an upper limit for the waiting time
	log.Info("[Wait]: waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, common.GetContainerNames(chaosDetails)...)
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		err := common.HelperFailedError(err, appLabel, experimentsDetails.ChaosNamespace, true)
		span.SetStatus(codes.Error, "failed to wait for completion of helper pod")
		span.RecordError(err)
		return err
	}

	//Deleting all the helper pod for container-kill chaos
	log.Info("[Cleanup]: Deleting all the helper pods")
	if err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		span.SetStatus(codes.Error, "failed to delete helper pod(s)")
		span.RecordError(err)
		return stacktrace.Propagate(err, "could not delete helper pod(s)")
	}

	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, targets, nodeName, runID string) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "CreateContainerKillFaultHelperPod")
	defer span.End()

	privilegedEnable := false
	if experimentsDetails.ContainerRuntime == "crio" {
		privilegedEnable = true
	}
	terminationGracePeriodSeconds := int64(experimentsDetails.TerminationGracePeriodSeconds)

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: experimentsDetails.ExperimentName + "-helper-",
			Namespace:    experimentsDetails.ChaosNamespace,
			Labels:       common.GetHelperLabels(chaosDetails.Labels, runID, experimentsDetails.ExperimentName),
			Annotations:  chaosDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			ServiceAccountName:            experimentsDetails.ChaosServiceAccount,
			ImagePullSecrets:              chaosDetails.ImagePullSecrets,
			RestartPolicy:                 apiv1.RestartPolicyNever,
			NodeName:                      nodeName,
			TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
			Volumes: []apiv1.Volume{
				{
					Name: "cri-socket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: experimentsDetails.SocketPath,
						},
					},
				},
			},
			Containers: []apiv1.Container{
				{
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"/bin/bash",
					},
					Args: []string{
						"-c",
						"./helpers -name container-kill",
					},
					Resources: chaosDetails.Resources,
					Env:       getPodEnv(ctx, experimentsDetails, targets),
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "cri-socket",
							MountPath: experimentsDetails.SocketPath,
						},
					},
					SecurityContext: &apiv1.SecurityContext{
						Privileged: &privilegedEnable,
					},
				},
			},
		},
	}

	if len(chaosDetails.SideCar) != 0 {
		helperPod.Spec.Containers = append(helperPod.Spec.Containers, common.BuildSidecar(chaosDetails)...)
		helperPod.Spec.Volumes = append(helperPod.Spec.Volumes, common.GetSidecarVolumes(chaosDetails)...)
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(context.Background(), helperPod, v1.CreateOptions{})
	if err != nil {
		span.SetStatus(codes.Error, "failed to create helper pod")
		err := cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("unable to create helper pod: %s", err.Error())}
		span.RecordError(err)
		return err
	}
	return nil
}

// getPodEnv derive all the env required for the helper pod
func getPodEnv(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, targets string) []apiv1.EnvVar {

	var envDetails common.ENVDetails
	envDetails.SetEnv("TARGETS", targets).
		SetEnv("TOTAL_CHAOS_DURATION", strconv.Itoa(experimentsDetails.ChaosDuration)).
		SetEnv("CHAOS_NAMESPACE", experimentsDetails.ChaosNamespace).
		SetEnv("CHAOSENGINE", experimentsDetails.EngineName).
		SetEnv("CHAOS_UID", string(experimentsDetails.ChaosUID)).
		SetEnv("CHAOS_INTERVAL", strconv.Itoa(experimentsDetails.ChaosInterval)).
		SetEnv("SOCKET_PATH", experimentsDetails.SocketPath).
		SetEnv("CONTAINER_RUNTIME", experimentsDetails.ContainerRuntime).
		SetEnv("SIGNAL", experimentsDetails.Signal).
		SetEnv("STATUS_CHECK_DELAY", strconv.Itoa(experimentsDetails.Delay)).
		SetEnv("STATUS_CHECK_TIMEOUT", strconv.Itoa(experimentsDetails.Timeout)).
		SetEnv("EXPERIMENT_NAME", experimentsDetails.ExperimentName).
		SetEnv("INSTANCE_ID", experimentsDetails.InstanceID).
		SetEnv("OTEL_EXPORTER_OTLP_ENDPOINT", os.Getenv(telemetry.OTELExporterOTLPEndpoint)).
		SetEnv("TRACE_PARENT", telemetry.GetMarshalledSpanFromContext(ctx)).
		SetEnvFromDownwardAPI("v1", "metadata.name")

	return envDetails.ENV
}

// SetChaosTunables will setup a random value within a given range of values
// If the value is not provided in range it'll setup the initial provided value.
func SetChaosTunables(experimentsDetails *experimentTypes.ExperimentDetails) {
	experimentsDetails.PodsAffectedPerc = common.ValidateRange(experimentsDetails.PodsAffectedPerc)
	experimentsDetails.Sequence = common.GetRandomSequence(experimentsDetails.Sequence)
}
