package lib

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-io-stress/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrepareNodeIOStress contains preparation steps before chaos injection
func PrepareNodeIOStress(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PrepareNodeIOStressFault")
	defer span.End()
	//set up the tunables if provided in range
	setChaosTunables(experimentsDetails)

	log.InfoWithValues("[Info]: The details of chaos tunables are:", logrus.Fields{
		"FilesystemUtilizationBytes":      experimentsDetails.FilesystemUtilizationBytes,
		"FilesystemUtilizationPercentage": experimentsDetails.FilesystemUtilizationPercentage,
		"CPU Core":                        experimentsDetails.CPU,
		"NumberOfWorkers":                 experimentsDetails.NumberOfWorkers,
		"Node Affected Percentage":        experimentsDetails.NodesAffectedPerc,
		"Sequence":                        experimentsDetails.Sequence,
	})

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Select node for node-io-stress
	nodesAffectedPerc, _ := strconv.Atoi(experimentsDetails.NodesAffectedPerc)
	targetNodeList, err := common.GetNodeList(experimentsDetails.TargetNodes, experimentsDetails.NodeLabel, nodesAffectedPerc, clients)
	if err != nil {
		return stacktrace.Propagate(err, "could not get node list")
	}
	log.InfoWithValues("[Info]: Details of Nodes under chaos injection", logrus.Fields{
		"No. Of Nodes": len(targetNodeList),
		"Node Names":   targetNodeList,
	})

	if experimentsDetails.EngineName != "" {
		if err := common.SetHelperData(chaosDetails, experimentsDetails.SetHelperData, clients); err != nil {
			span.SetStatus(codes.Error, "could not set helper data")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not set helper data")
		}
	}

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(ctx, experimentsDetails, targetNodeList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			span.SetStatus(codes.Error, "could not run chaos in serial mode")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err = injectChaosInParallelMode(ctx, experimentsDetails, targetNodeList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			span.SetStatus(codes.Error, "could not run chaos in parallel mode")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		span.SetStatus(codes.Error, "sequence not supported")
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

// injectChaosInSerialMode stress the io of all the target nodes serially (one by one)
func injectChaosInSerialMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, targetNodeList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectNodeIOStressFaultInSerialMode")
	defer span.End()

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	for _, appNode := range targetNodeList {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNode + " node"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		log.InfoWithValues("[Info]: Details of Node under chaos injection", logrus.Fields{
			"NodeName":                        appNode,
			"FilesystemUtilizationPercentage": experimentsDetails.FilesystemUtilizationPercentage,
			"NumberOfWorkers":                 experimentsDetails.NumberOfWorkers,
		})

		experimentsDetails.RunID = stringutils.GetRunID()

		// Creating the helper pod to perform node io stress
		if err := createHelperPod(ctx, experimentsDetails, chaosDetails, appNode, clients); err != nil {
			span.SetStatus(codes.Error, "could not create helper pod")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not create helper pod")
		}

		appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, experimentsDetails.RunID)

		//Checking the status of helper pod
		log.Info("[Status]: Checking the status of the helper pod")
		if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			span.SetStatus(codes.Error, "could not check helper status")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not check helper status")
		}
		common.SetTargets(appNode, "injected", "node", chaosDetails)

		log.Info("[Wait]: Waiting till the completion of the helper pod")
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.ExperimentName)
		common.SetTargets(appNode, "reverted", "node", chaosDetails)
		if err != nil || podStatus == "Failed" {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			err := common.HelperFailedError(err, appLabel, experimentsDetails.ChaosNamespace, false)
			span.SetStatus(codes.Error, "helper pod failed")
			span.RecordError(err)
			return err
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		if err := common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			span.SetStatus(codes.Error, "could not delete helper pod(s)")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not delete helper pod(s)")
		}
	}
	return nil
}

// injectChaosInParallelMode stress the io of all the target nodes in parallel mode (all at once)
func injectChaosInParallelMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, targetNodeList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectNodeIOStressFaultInParallelMode")
	defer span.End()

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			span.SetStatus(codes.Error, "probe failed")
			span.RecordError(err)
			return err
		}
	}

	experimentsDetails.RunID = stringutils.GetRunID()

	for _, appNode := range targetNodeList {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNode + " node"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		log.InfoWithValues("[Info]: Details of Node under chaos injection", logrus.Fields{
			"NodeName":                        appNode,
			"FilesystemUtilizationPercentage": experimentsDetails.FilesystemUtilizationPercentage,
			"NumberOfWorkers":                 experimentsDetails.NumberOfWorkers,
		})

		// Creating the helper pod to perform node io stress
		if err := createHelperPod(ctx, experimentsDetails, chaosDetails, appNode, clients); err != nil {
			span.SetStatus(codes.Error, "could not create helper pod")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not create helper pod")
		}
	}

	appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, experimentsDetails.RunID)

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		span.SetStatus(codes.Error, "could not check helper status")
		span.RecordError(err)
		return stacktrace.Propagate(err, "could not check helper status")
	}

	for _, appNode := range targetNodeList {
		common.SetTargets(appNode, "injected", "node", chaosDetails)
	}

	log.Info("[Wait]: Waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, common.GetContainerNames(chaosDetails)...)
	for _, appNode := range targetNodeList {
		common.SetTargets(appNode, "reverted", "node", chaosDetails)
	}
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		err := common.HelperFailedError(err, appLabel, chaosDetails.ChaosNamespace, false)
		span.SetStatus(codes.Error, "helper pod failed")
		span.RecordError(err)
		return err
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	if err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		span.SetStatus(codes.Error, "could not delete helper pod(s)")
		span.RecordError(err)
		return stacktrace.Propagate(err, "could not delete helper pod(s)")
	}

	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, chaosDetails *types.ChaosDetails, appNode string, clients clients.ClientSets) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "CreateNodeIOStressFaultHelperPod")
	defer span.End()
	terminationGracePeriodSeconds := int64(experimentsDetails.TerminationGracePeriodSeconds)

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: experimentsDetails.ExperimentName + "-helper-",
			Namespace:    experimentsDetails.ChaosNamespace,
			Labels:       common.GetHelperLabels(chaosDetails.Labels, experimentsDetails.RunID, experimentsDetails.ExperimentName),
			Annotations:  chaosDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy:                 apiv1.RestartPolicyNever,
			ImagePullSecrets:              chaosDetails.ImagePullSecrets,
			NodeName:                      appNode,
			TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
			Containers: []apiv1.Container{
				{
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"stress-ng",
					},
					Args:      getContainerArguments(experimentsDetails),
					Resources: chaosDetails.Resources,
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
		span.SetStatus(codes.Error, "could not create helper pod")
		err := cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("unable to create helper pod: %s", err.Error())}
		span.RecordError(err)
		return err
	}
	return nil
}

// getContainerArguments derives the args for the pumba stress helper pod
func getContainerArguments(experimentsDetails *experimentTypes.ExperimentDetails) []string {

	var hddbytes string
	if experimentsDetails.FilesystemUtilizationBytes == "0" {
		if experimentsDetails.FilesystemUtilizationPercentage == "0" {
			hddbytes = "10%"
			log.Info("Neither of FilesystemUtilizationPercentage or FilesystemUtilizationBytes provided, proceeding with a default FilesystemUtilizationPercentage value of 10%")
		} else {
			hddbytes = experimentsDetails.FilesystemUtilizationPercentage + "%"
		}
	} else {
		if experimentsDetails.FilesystemUtilizationPercentage == "0" {
			hddbytes = experimentsDetails.FilesystemUtilizationBytes + "G"
		} else {
			hddbytes = experimentsDetails.FilesystemUtilizationPercentage + "%"
			log.Warn("Both FsUtilPercentage & FsUtilBytes provided as inputs, using the FsUtilPercentage value to proceed with stress exp")
		}
	}

	stressArgs := []string{
		"--cpu",
		experimentsDetails.CPU,
		"--vm",
		experimentsDetails.VMWorkers,
		"--io",
		experimentsDetails.NumberOfWorkers,
		"--hdd",
		experimentsDetails.NumberOfWorkers,
		"--hdd-bytes",
		hddbytes,
		"--timeout",
		strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
		"--temp-path",
		"/tmp",
	}
	return stressArgs
}

// setChaosTunables will set up a random value within a given range of values
// If the value is not provided in range it'll set up the initial provided value.
func setChaosTunables(experimentsDetails *experimentTypes.ExperimentDetails) {
	experimentsDetails.FilesystemUtilizationBytes = common.ValidateRange(experimentsDetails.FilesystemUtilizationBytes)
	experimentsDetails.FilesystemUtilizationPercentage = common.ValidateRange(experimentsDetails.FilesystemUtilizationPercentage)
	experimentsDetails.CPU = common.ValidateRange(experimentsDetails.CPU)
	experimentsDetails.VMWorkers = common.ValidateRange(experimentsDetails.VMWorkers)
	experimentsDetails.NumberOfWorkers = common.ValidateRange(experimentsDetails.NumberOfWorkers)
	experimentsDetails.NodesAffectedPerc = common.ValidateRange(experimentsDetails.NodesAffectedPerc)
	experimentsDetails.Sequence = common.GetRandomSequence(experimentsDetails.Sequence)
}
