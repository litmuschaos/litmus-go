package lib

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/instance-runscript/types"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	azureCommon "github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"
	azureStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure/instance"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/palantir/stacktrace"
	"go.opentelemetry.io/otel"
)

var (
	err           error
	inject, abort chan os.Signal
)

// PrepareAzureRunScript will initialize instanceNameList and start chaos injection based on sequence method selected
func PrepareAzureRunScript(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PrepareAzureInstanceRunScriptFault")
	defer span.End()

	// inject channel is used to transmit signal notifications
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	// Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//  get the instance name or list of instance names
	instanceNameList := strings.Split(experimentsDetails.AzureInstanceNames, ",")
	if experimentsDetails.AzureInstanceNames == "" || len(instanceNameList) == 0 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "no instance name found to stop"}
	}

	// watching for the abort signal and revert the chaos
	go abortWatcher(experimentsDetails, instanceNameList)

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(ctx, experimentsDetails, instanceNameList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err = injectChaosInParallelMode(ctx, experimentsDetails, instanceNameList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
	}

	// Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// injectChaosInSerialMode will inject the Azure instance termination in serial mode that is one after the other
func injectChaosInSerialMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, instanceNameList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectAzureInstanceRunScriptFaultInSerialMode")
	defer span.End()

	select {
	case <-inject:
		//running script the chaos execution, if abort signal received
		os.Exit(0)
	default:
		// ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

			log.Infof("[Info]: Target instanceName list, %v", instanceNameList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on Azure instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			//Run script in the instance serially
			for i, vmName := range instanceNameList {

				//Running start Script the Azure instance
				log.Infof("[Chaos]:Running Script the Azure instance: %v", vmName)
				if experimentsDetails.ScaleSet == "enable" {
					return notImplementedError("scale set instance run script not implemented")
				} else {
					if err := azureStatus.AzureInstanceRunScript(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName, experimentDetails.PowershellChaosStartBase64OrPsFilePath, experimentDetails.IsBase64, experimentDetails.PowershellChaosStartParams); err != nil {
						return stacktrace.Propagate(err, "unable to run script in the Azure instance")
					}
				}

				// Run the probes during chaos
				// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
				if len(resultDetails.ProbeDetails) != 0 && i == 0 {
					if err = probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
						return stacktrace.Propagate(err, "failed to run probes")
					}
				}

				// Wait for Chaos interval
				log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
				common.WaitForDuration(experimentsDetails.ChaosInterval)

				// Running the end PS Script in the Azure instance
				log.Info("[Chaos]: Starting back the Azure instance")
				if experimentsDetails.ScaleSet == "enable" {
					return notImplementedError("scale set instance run script not implemented")
				} else {
					if err := azureStatus.AzureInstanceRunScript(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName, experimentDetails.PowershellChaosEndBase64OrPsFilePath, experimentDetails.IsBase64, experimentDetails.PowershellChaosEndParams); err != nil {
						return stacktrace.Propagate(err, "unable to run the script in the Azure instance")
					}
				}
			}
			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}
	return nil
}

// injectChaosInParallelMode will inject the Azure instance termination in parallel mode that is all at once
func injectChaosInParallelMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, instanceNameList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectAzureInstanceRunScriptFaultInParallelMode")
	defer span.End()
	return notImplementedError("scale set instance run script not implemented so parallel mode run script also not implemented.")
}

// watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, instanceNameList []string) {
	<-abort

	return notImplementedError("Abort run script not implemented")
}
