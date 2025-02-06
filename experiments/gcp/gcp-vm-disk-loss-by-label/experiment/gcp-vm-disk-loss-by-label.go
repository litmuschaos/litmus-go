package experiment

import (
	"context"
	"os"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/gcp-vm-disk-loss-by-label/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/cloud/gcp"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-disk-loss/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-disk-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/compute/v1"
)

// GCPVMDiskLossByLabel contains steps to inject chaos
func GCPVMDiskLossByLabel(ctx context.Context, clients clients.ClientSets) {
	span := trace.SpanFromContext(ctx)

	var (
		computeService *compute.Service
		err            error
	)

	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}

	//Fetching all the ENV passed from the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", os.Getenv("EXPERIMENT_NAME"))
	experimentEnv.GetENV(&experimentsDetails)

	// Initialize the chaos attributes
	types.InitialiseChaosVariables(&chaosDetails)

	// Initialize Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	if experimentsDetails.EngineName != "" {
		// Get values from chaosengine. Bail out upon error, as we haven't entered exp business logic yet
		if err := types.GetValuesFromChaosEngine(&chaosDetails, clients, &resultDetails); err != nil {
			log.Errorf("Unable to initialize the probes, err: %v", err)
			span.SetStatus(codes.Error, "Unable to initialize the probes")
			span.RecordError(err)
			return
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	if err := result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT"); err != nil {
		log.Errorf("Unable to Create the Chaos Result, err: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Unable to create the chaosresult")
		span.RecordError(err)
		return
	}

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	// generating the event in chaosresult to marked the verdict as awaited
	msg := "experiment: " + experimentsDetails.ExperimentName + ", Result: Awaited"
	types.SetResultEventAttributes(&eventsDetails, types.AwaitedVerdict, msg, "Normal", &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("[Info]: The disk information is as follows", logrus.Fields{
		"Disk Volume Label": experimentsDetails.DiskVolumeLabel,
		"Zones":             experimentsDetails.Zones,
		"Sequence":          experimentsDetails.Sequence,
	})

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go common.AbortWatcher(experimentsDetails.ExperimentName, clients, &resultDetails, &chaosDetails, &eventsDetails)

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the pre-chaos check
		if len(resultDetails.ProbeDetails) != 0 {

			if err := probe.RunProbes(ctx, &chaosDetails, clients, &resultDetails, "PreChaos", &eventsDetails); err != nil {
				log.Errorf("Probe Failed, err: %v", err)
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Warning", &chaosDetails)
				events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
				span.SetStatus(codes.Error, "Probe Failed")
				span.RecordError(err)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}
		// generating the events for the pre-chaos check
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// Create a compute service to access the compute engine resources
	computeService, err = gcp.GetGCPComputeService()
	if err != nil {
		log.Errorf("Failed to obtain a gcp compute service, err: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Failed to obtain a gcp compute service")
		span.RecordError(err)
		return
	}

	//selecting the target instances (pre-chaos)
	if err := gcp.SetTargetDiskVolumes(computeService, &experimentsDetails); err != nil {
		log.Errorf("Failed to get the target gcp disk volumes, err: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Failed to get the target gcp disk volumes")
		span.RecordError(err)
		return
	}

	log.Info("[Status]: Disk volumes are attached to the VM instances (pre-chaos)")

	chaosDetails.Phase = types.ChaosInjectPhase

	if err := litmusLIB.PrepareDiskVolumeLossByLabel(ctx, computeService, &experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails); err != nil {
		log.Errorf("Chaos injection failed, err: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Chaos injection failed")
		span.RecordError(err)
		return
	}

	log.Infof("[Confirmation]: %v chaos has been injected successfully", experimentsDetails.ExperimentName)
	resultDetails.Verdict = v1alpha1.ResultVerdictPassed

	chaosDetails.Phase = types.PostChaosPhase

	// Checking disk volume attachment post-chaos
	for i := range experimentsDetails.TargetDiskVolumeNamesList {
		instanceName, err := gcp.GetVolumeAttachmentDetails(computeService, experimentsDetails.GCPProjectID, experimentsDetails.Zones, experimentsDetails.TargetDiskVolumeNamesList[i])
		if err != nil || instanceName == "" {
			log.Errorf("Failed to verify disk volume attachment status, err: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "Failed to verify disk volume attachment status")
			span.RecordError(err)
			return
		}
	}

	log.Info("[Status]: Disk volumes are attached to the VM instances (post-chaos)")

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			if err := probe.RunProbes(ctx, &chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails); err != nil {
				log.Errorf("Probes Failed, err: %v", err)
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Warning", &chaosDetails)
				events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
				span.SetStatus(codes.Error, "Probes Failed")
				span.RecordError(err)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}

		// generating post chaos event
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	//Updating the chaosResult in the end of experiment
	log.Infof("[The End]: Updating the chaos result of %v experiment (EOT)", experimentsDetails.ExperimentName)
	if err := result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT"); err != nil {
		log.Errorf("Unable to Update the Chaos Result, err: %v", err)
		span.SetStatus(codes.Error, "Unable to Update the Chaos Result")
		span.RecordError(err)
		return
	}

	// generating the event in chaosresult to marked the verdict as pass/fail
	msg = "experiment: " + experimentsDetails.ExperimentName + ", Result: " + string(resultDetails.Verdict)
	reason, eventType := types.GetChaosResultVerdictEvent(resultDetails.Verdict)
	types.SetResultEventAttributes(&eventsDetails, reason, msg, eventType, &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	if experimentsDetails.EngineName != "" {
		msg := experimentsDetails.ExperimentName + " experiment has been " + string(resultDetails.Verdict) + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}
}
