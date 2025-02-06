package lib

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/load/k6-loadgen/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
	"github.com/palantir/stacktrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func experimentExecution(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectK6LoadGenFault")
	defer span.End()

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}
	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			span.SetStatus(codes.Error, "Probe failed")
			span.RecordError(err)
			return err
		}
	}

	runID := stringutils.GetRunID()

	// creating the helper pod to perform k6-loadgen chaos
	if err := createHelperPod(ctx, experimentsDetails, clients, chaosDetails, runID); err != nil {
		span.SetStatus(codes.Error, "could not create helper pod")
		span.RecordError(err)
		return stacktrace.Propagate(err, "could not create helper pod")
	}

	appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, runID)

	//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		span.SetStatus(codes.Error, "could not check helper status")
		span.RecordError(err)
		return stacktrace.Propagate(err, "could not check helper status")
	}

	// Wait till the completion of the helper pod
	// set an upper limit for the waiting time
	log.Info("[Wait]: Waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, common.GetContainerNames(chaosDetails)...)
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		err := common.HelperFailedError(err, appLabel, experimentsDetails.ChaosNamespace, true)
		span.SetStatus(codes.Error, "helper pod failed")
		span.RecordError(err)
		return err
	}

	//Deleting all the helper pod for k6-loadgen chaos
	log.Info("[Cleanup]: Deleting all the helper pods")
	if err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		span.SetStatus(codes.Error, "could not delete helper pod(s)")
		span.RecordError(err)
		return stacktrace.Propagate(err, "could not delete helper pod(s)")
	}

	return nil
}

// PrepareChaos contains the preparation steps before chaos injection
func PrepareChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PrepareK6LoadGenFault")
	defer span.End()

	// Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// Starting the k6-loadgen experiment
	if err := experimentExecution(ctx, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
		span.SetStatus(codes.Error, "could not execute chaos")
		span.RecordError(err)
		return stacktrace.Propagate(err, "could not execute chaos")
	}

	// Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, runID string) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "CreateK6LoadGenFaultHelperPod")
	defer span.End()

	const volumeName = "script-volume"
	const mountPath = "/mnt"

	var envs []corev1.EnvVar
	args := []string{
		mountPath + "/" + experimentsDetails.ScriptSecretKey,
		"-q",
		"--duration",
		strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
		"--tag",
		"trace_id=" + span.SpanContext().TraceID().String(),
	}

	if otelExporterEndpoint := os.Getenv(telemetry.OTELExporterOTLPEndpoint); otelExporterEndpoint != "" {
		envs = []corev1.EnvVar{
			{
				Name:  "K6_OTEL_METRIC_PREFIX",
				Value: experimentsDetails.OTELMetricPrefix,
			},
			{
				Name:  "K6_OTEL_GRPC_EXPORTER_INSECURE",
				Value: "true",
			},
			{
				Name:  "K6_OTEL_GRPC_EXPORTER_ENDPOINT",
				Value: otelExporterEndpoint,
			},
		}
		args = append(args, "--out", "experimental-opentelemetry")
	}

	helperPod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: experimentsDetails.ExperimentName + "-helper-",
			Namespace:    experimentsDetails.ChaosNamespace,
			Labels:       common.GetHelperLabels(chaosDetails.Labels, runID, experimentsDetails.ExperimentName),
			Annotations:  chaosDetails.Annotations,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:    corev1.RestartPolicyNever,
			ImagePullSecrets: chaosDetails.ImagePullSecrets,
			Containers: []corev1.Container{
				{
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: corev1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"k6",
						"run",
					},
					Args:      args,
					Env:       envs,
					Resources: chaosDetails.Resources,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      volumeName,
							MountPath: mountPath,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: volumeName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: experimentsDetails.ScriptSecretName,
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(context.Background(), helperPod, v1.CreateOptions{})
	if err != nil {
		span.SetStatus(codes.Error, "could not create helper pod")
		err := cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("unable to create helper pod: %s", err.Error())}
		span.RecordError(err)
		return err
	}
	return nil
}
