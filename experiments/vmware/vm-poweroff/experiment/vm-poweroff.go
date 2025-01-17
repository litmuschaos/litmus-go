package experiment

import (
	"context"
	"os"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/vm-poweroff/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/cloud/vmware"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/types"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/sirupsen/logrus"
)

var err error

// VMPoweroff contains steps to inject vm-power-off chaos
func VMPoweroff(ctx context.Context, clients clients.ClientSets) {
	span := trace.SpanFromContext(ctx)

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
			log.Errorf("Unable to initialize the probes: %v", err)
			span.SetStatus(codes.Error, "Unable to initialize the probes")
			span.RecordError(err)
			return
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	if err := result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT"); err != nil {
		log.Errorf("Unable to create the chaosresult: %v", err)
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
	if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult"); eventErr != nil {
		log.Errorf("Failed to create %v event inside chaosresult", types.AwaitedVerdict)
	}

	if experimentsDetails.VMTag != "" {
		// GET VM IDs FROM TAG
		experimentsDetails.VMIds, err = vmware.GetVMIDFromTag(experimentsDetails.VMTag)
		if err != nil {
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			log.Errorf("Unable to get the VM ID, err: %v", err)
			span.SetStatus(codes.Error, "Unable to get the VM ID")
			span.RecordError(err)
			return
		}
	}

	//DISPLAY THE VM INFORMATION
	log.InfoWithValues("[Info]: The Instance information is as follows", logrus.Fields{
		"VM MOIDS":       experimentsDetails.VMIds,
		"Ramp Time":      experimentsDetails.RampTime,
		"Chaos Duration": experimentsDetails.ChaosDuration,
	})

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go common.AbortWatcherWithoutExit(experimentsDetails.ExperimentName, clients, &resultDetails, &chaosDetails, &eventsDetails)

	// GET SESSION ID TO LOGIN TO VCENTER
	cookie, err := vmware.GetVcenterSessionID(experimentsDetails.VcenterServer, experimentsDetails.VcenterUser, experimentsDetails.VcenterPass)
	if err != nil {
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		log.Errorf("Vcenter Login failed: %v", err)
		span.SetStatus(codes.Error, "Vcenter Login failed")
		span.RecordError(err)
		return
	}

	if chaosDetails.DefaultHealthCheck {
		// PRE-CHAOS VM STATUS CHECK
		if err := vmware.VMStatusCheck(experimentsDetails.VcenterServer, experimentsDetails.VMIds, cookie); err != nil {
			log.Errorf("VM status check failed: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "VM status check failed")
			span.RecordError(err)
			return
		}
		log.Info("[Verification]: VMs are in running state (pre-chaos)")
	}

	if experimentsDetails.EngineName != "" {
		// marking IUT as running, as we already checked the status of instance under test
		msg := "IUT: Running"

		// run the probes in the pre-chaos check
		if len(resultDetails.ProbeDetails) != 0 {

			if err = probe.RunProbes(ctx, &chaosDetails, clients, &resultDetails, "PreChaos", &eventsDetails); err != nil {
				log.Errorf("Probe Failed: %v", err)
				msg := "IUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Warning", &chaosDetails)
				if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
					log.Errorf("Failed to create %v event inside chaosengine", types.PreChaosCheck)
				}
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
				span.SetStatus(codes.Error, "Probe Failed")
				span.RecordError(err)
				return
			}
			msg = "IUT: Running, Probes: Successful"
		}
		// generating the events for the pre-chaos check
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Normal", &chaosDetails)
		if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
			log.Errorf("Failed to create %v event inside chaosengine", types.PreChaosCheck)
		}
	}

	chaosDetails.Phase = types.ChaosInjectPhase

	if err = litmusLIB.InjectVMPowerOffChaos(ctx, &experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails, cookie); err != nil {
		log.Errorf("Chaos injection failed: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Chaos injection failed")
		span.RecordError(err)
		return
	}

	log.Infof("[Confirmation]: %v chaos has been injected successfully", experimentsDetails.ExperimentName)
	resultDetails.Verdict = v1alpha1.ResultVerdictPassed

	chaosDetails.Phase = types.PostChaosPhase

	if chaosDetails.DefaultHealthCheck {
		//POST-CHAOS VM STATUS CHECK
		log.Info("[Status]: Verify that the IUT (Instance Under Test) is running (post-chaos)")
		if err := vmware.VMStatusCheck(experimentsDetails.VcenterServer, experimentsDetails.VMIds, cookie); err != nil {
			log.Errorf("VM status check failed: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "VM status check failed")
			span.RecordError(err)
			return
		}
		log.Info("[Verification]: VMs are in running state (post-chaos)")
	}

	if experimentsDetails.EngineName != "" {
		// marking IUT as running, as we already checked the status of instance under test
		msg := "IUT: Running"

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			if err = probe.RunProbes(ctx, &chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails); err != nil {
				log.Errorf("Probes Failed: %v", err)
				msg := "IUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Warning", &chaosDetails)
				if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
					log.Errorf("Failed to create %v event inside chaosengine", types.PostChaosCheck)
				}
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
				span.SetStatus(codes.Error, "Probes Failed")
				span.RecordError(err)
				return
			}
			msg = "IUT: Running, Probes: Successful"
		}

		// generating post chaos event
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Normal", &chaosDetails)
		if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
			log.Errorf("Failed to create %v event inside chaosengine", types.PostChaosCheck)
		}
	}

	//Updating the chaosResult in the end of experiment
	log.Infof("[The End]: Updating the chaos result of %v experiment (EOT)", experimentsDetails.ExperimentName)
	if err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT"); err != nil {
		log.Errorf("Unable to update the chaosresult: %v", err)
		span.SetStatus(codes.Error, "Unable to update the chaosresult")
		span.RecordError(err)
		return
	}

	// generating the event in chaosresult to marked the verdict as pass/fail
	msg = "experiment: " + experimentsDetails.ExperimentName + ", Result: " + string(resultDetails.Verdict)
	reason, eventType := types.GetChaosResultVerdictEvent(resultDetails.Verdict)
	types.SetResultEventAttributes(&eventsDetails, reason, msg, eventType, &resultDetails)
	if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult"); eventErr != nil {
		log.Errorf("Failed to create %v event inside chaosresult", reason)
	}
	if experimentsDetails.EngineName != "" {
		msg := experimentsDetails.ExperimentName + " experiment has been " + string(resultDetails.Verdict) + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
			log.Errorf("Failed to create %v event inside chaosengine", types.Summary)
		}
	}
}
