package experiment

import (
	"context"
	"os"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/azure-instance-stop/lib"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/azure/instance-stop/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/instance-stop/types"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	azureCommon "github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"
	azureStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure/instance"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
)

// AzureInstanceStop inject the azure instance stop chaos
func AzureInstanceStop(ctx context.Context, clients clients.ClientSets) {
	span := trace.SpanFromContext(ctx)

	var err error
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
		if err = types.GetValuesFromChaosEngine(&chaosDetails, clients, &resultDetails); err != nil {
			log.Errorf("Unable to initialize the probes: %v", err)
			span.SetStatus(codes.Error, "Unable to initialize the probes")
			span.RecordError(err)
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to create the chaosresult: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Unable to create the chaosresult")
		span.RecordError(err)
		return
	}

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go common.AbortWatcherWithoutExit(experimentsDetails.ExperimentName, clients, &resultDetails, &chaosDetails, &eventsDetails)

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("The instance information is as follows", logrus.Fields{
		"Chaos Duration": experimentsDetails.ChaosDuration,
		"Resource Group": experimentsDetails.ResourceGroup,
		"Instance Name":  experimentsDetails.AzureInstanceNames,
		"Sequence":       experimentsDetails.Sequence,
	})

	// Setting up Azure Subscription ID
	if experimentsDetails.SubscriptionID, err = azureCommon.GetSubscriptionID(); err != nil {
		log.Errorf("Failed to get the subscription id: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "fail to get the subscription id")
		span.RecordError(err)
		return
	}

	// generating the event in chaosresult to marked the verdict as awaited
	msg := "experiment: " + experimentsDetails.ExperimentName + ", Result: Awaited"
	types.SetResultEventAttributes(&eventsDetails, types.AwaitedVerdict, msg, "Normal", &resultDetails)
	if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResults"); eventErr != nil {
		log.Errorf("Failed to create %v event inside chaosresults", types.AwaitedVerdict)
	}

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the pre-chaos check
		if len(resultDetails.ProbeDetails) != 0 {

			err = probe.RunProbes(ctx, &chaosDetails, clients, &resultDetails, "PreChaos", &eventsDetails)
			if err != nil {
				log.Errorf("Probe Failed: %v", err)
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Warning", &chaosDetails)
				if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
					log.Errorf("Failed to create %v event inside chaosengine", types.PreChaosCheck)
				}
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
				span.SetStatus(codes.Error, "Probe Failed")
				span.RecordError(err)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}
		// generating the events for the pre-chaos check
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Normal", &chaosDetails)
		if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
			log.Errorf("Failed to create %v event inside chaosengine", types.PreChaosCheck)
		}
	}

	//Verify the azure target instance is running (pre-chaos)
	if chaosDetails.DefaultHealthCheck {
		if err = azureStatus.InstanceStatusCheckByName(experimentsDetails.AzureInstanceNames, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup); err != nil {
			log.Errorf("Azure instance status check failed: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "Azure instance status check failed")
			span.RecordError(err)
			return
		}
		log.Info("[Status]: Azure instance(s) is in running state (pre-chaos)")
	}

	chaosDetails.Phase = types.ChaosInjectPhase

	if err = litmusLIB.PrepareAzureStop(ctx, &experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails); err != nil {
		log.Errorf("Chaos injection failed: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Chaos injection failed")
		span.RecordError(err)
		return
	}

	log.Info("[Confirmation]: Azure instance stop chaos has been injected successfully")
	resultDetails.Verdict = v1alpha1.ResultVerdictPassed

	chaosDetails.Phase = types.PostChaosPhase

	//Verify the azure instance is running (post chaos)
	if chaosDetails.DefaultHealthCheck {
		if err = azureStatus.InstanceStatusCheckByName(experimentsDetails.AzureInstanceNames, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup); err != nil {
			log.Errorf("Azure instance status check failed: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "Azure instance status check failed")
			span.RecordError(err)
			return
		}
		log.Info("[Status]: Azure instance is in running state (post chaos)")
	}

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			err = probe.RunProbes(ctx, &chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails)
			if err != nil {
				log.Errorf("Probes Failed: %v", err)
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Warning", &chaosDetails)
				if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
					log.Errorf("Failed to create %v event inside chaosengine", types.PostChaosCheck)
				}
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
				span.SetStatus(codes.Error, "Probes Failed")
				span.RecordError(err)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}

		// generating post chaos event
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Normal", &chaosDetails)
		if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
			log.Errorf("Failed to create %v event inside chaosengine", types.PostChaosCheck)
		}
	}

	//Updating the chaosResult in the end of experiment
	log.Infof("[The End]: Updating the chaos result of %v experiment (EOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
	if err != nil {
		log.Errorf("Unable to update the chaosresult:  %v", err)
	}

	// generating the event in chaosresult to marked the verdict as pass/fail
	msg = "experiment: " + experimentsDetails.ExperimentName + ", Result: " + string(resultDetails.Verdict)
	reason, eventType := types.GetChaosResultVerdictEvent(resultDetails.Verdict)
	types.SetResultEventAttributes(&eventsDetails, reason, msg, eventType, &resultDetails)
	if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResults"); eventErr != nil {
		log.Errorf("Failed to create %v event inside chaosresults", reason)
	}

	if experimentsDetails.EngineName != "" {
		msg := experimentsDetails.ExperimentName + " experiment has been " + string(resultDetails.Verdict) + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
			log.Errorf("Failed to create %v event inside chaosengine", types.Summary)
		}
	}

}
