package lib

import (
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/load/locust-loadgen/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
	"github.com/palantir/stacktrace"
	"go.opentelemetry.io/otel"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
)

func experimentExecution(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectLocustLoadGenFault")
	defer span.End()

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on target pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	runID := stringutils.GetRunID()

	// creating the helper pod to perform locust-loadgen chaos
	if err := createHelperPod(ctx, experimentsDetails, clients, chaosDetails, runID); err != nil {
		return stacktrace.Propagate(err, "could not create helper pod")
	}

	appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, runID)

	//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return stacktrace.Propagate(err, "could not check helper status")
	}

	// Wait till the completion of the helper pod
	// set an upper limit for the waiting time
	log.Info("[Wait]: waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, common.GetContainerNames(chaosDetails)...)
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return common.HelperFailedError(err, appLabel, experimentsDetails.ChaosNamespace, true)
	}

	//Deleting all the helper pod for locust-loadgen chaos
	log.Info("[Cleanup]: Deleting all the helper pods")
	if err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return stacktrace.Propagate(err, "could not delete helper pod(s)")
	}

	return nil
}

// PrepareChaos contains the preparation steps before chaos injection
func PrepareChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PrepareLocustLoadGenFault")
	defer span.End()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	//Starting the locust-loadgen experiment
	if err := experimentExecution(ctx, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
		return stacktrace.Propagate(err, "could not execute chaos")
	}
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, runID string) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "CreateLocustLoadGenFaultHelperPod")
	defer span.End()

	const volumeName = "script-volume"
	const mountPath = "/mnt"

	args := []string{
		"--users", strconv.Itoa(experimentsDetails.Users),
		"--spawn-rate", strconv.Itoa(experimentsDetails.SpawnRate),
		"-H", experimentsDetails.Host,
		"-t", strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
	}

	if experimentsDetails.ConfigMapName != "" {
		args = append(args, "-f", mountPath+"/config.py")
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
						"locust",
						"--headless",
					},
					Args:      args,
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
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: experimentsDetails.ConfigMapName,
							},
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(context.Background(), helperPod, v1.CreateOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("unable to create helper pod: %s", err.Error())}
	}
	return nil
}
