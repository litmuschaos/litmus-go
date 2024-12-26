package lib

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/instance-stop/types"
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
	"go.opentelemetry.io/otel/codes"
)

var (
	err           error
	inject, abort chan os.Signal
)

// PrepareAzureStop will initialize instanceNameList and start chaos injection based on sequence method selected
func PrepareAzureStop(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PrepareAzureInstanceStopFault")
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
			span.SetStatus(codes.Error, "failed to run chaos in serial mode")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err = injectChaosInParallelMode(ctx, experimentsDetails, instanceNameList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			span.SetStatus(codes.Error, "failed to run chaos in parallel mode")
			span.RecordError(err)
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		span.SetStatus(codes.Error, "sequence not supported")
		err := cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
		span.RecordError(err)
		return err
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
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectAzureInstanceStopFaultInSerialMode")
	defer span.End()

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
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

			// PowerOff the instance serially
			for i, vmName := range instanceNameList {

				// Stopping the Azure instance
				log.Infof("[Chaos]: Stopping the Azure instance: %v", vmName)
				if experimentsDetails.ScaleSet == "enable" {
					if err := azureStatus.AzureScaleSetInstanceStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						span.SetStatus(codes.Error, "failed to stop the Azure instance")
						span.RecordError(err)
						return stacktrace.Propagate(err, "unable to stop the Azure instance")
					}
				} else {
					if err := azureStatus.AzureInstanceStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						span.SetStatus(codes.Error, "failed to stop the Azure instance")
						span.RecordError(err)
						return stacktrace.Propagate(err, "unable to stop the Azure instance")
					}
				}

				// Wait for Azure instance to completely stop
				log.Infof("[Wait]: Waiting for Azure instance '%v' to get in the stopped state", vmName)
				if err := azureStatus.WaitForAzureComputeDown(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					span.SetStatus(codes.Error, "failed to check instance poweroff status")
					span.RecordError(err)
					return stacktrace.Propagate(err, "instance poweroff status check failed")
				}

				// Run the probes during chaos
				// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
				if len(resultDetails.ProbeDetails) != 0 && i == 0 {
					if err = probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
						span.SetStatus(codes.Error, "failed to run probes")
						span.RecordError(err)
						return stacktrace.Propagate(err, "failed to run probes")
					}
				}

				// Wait for Chaos interval
				log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
				common.WaitForDuration(experimentsDetails.ChaosInterval)

				// Starting the Azure instance
				log.Info("[Chaos]: Starting back the Azure instance")
				if experimentsDetails.ScaleSet == "enable" {
					if err := azureStatus.AzureScaleSetInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						span.SetStatus(codes.Error, "failed to start the Azure instance")
						span.RecordError(err)
						return stacktrace.Propagate(err, "unable to start the Azure instance")
					}
				} else {
					if err := azureStatus.AzureInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						span.SetStatus(codes.Error, "failed to start the Azure instance")
						span.RecordError(err)
						return stacktrace.Propagate(err, "unable to start the Azure instance")
					}
				}

				// Wait for Azure instance to get in running state
				log.Infof("[Wait]: Waiting for Azure instance '%v' to get in the running state", vmName)
				if err := azureStatus.WaitForAzureComputeUp(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					span.SetStatus(codes.Error, "failed to check instance power on status")
					span.RecordError(err)
					return stacktrace.Propagate(err, "instance power on status check failed")
				}
			}
			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}
	return nil
}

// injectChaosInParallelMode will inject the Azure instance termination in parallel mode that is all at once
func injectChaosInParallelMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, instanceNameList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectAzureInstanceStopFaultInParallelMode")
	defer span.End()

	select {
	case <-inject:
		// Stopping the chaos execution, if abort signal received
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

			// PowerOff the instances parallelly
			for _, vmName := range instanceNameList {
				// Stopping the Azure instance
				log.Infof("[Chaos]: Stopping the Azure instance: %v", vmName)
				if experimentsDetails.ScaleSet == "enable" {
					if err := azureStatus.AzureScaleSetInstanceStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						span.SetStatus(codes.Error, "failed to stop the Azure instance")
						span.RecordError(err)
						return stacktrace.Propagate(err, "unable to stop Azure instance")
					}
				} else {
					if err := azureStatus.AzureInstanceStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						span.SetStatus(codes.Error, "failed to stop the Azure instance")
						span.RecordError(err)
						return stacktrace.Propagate(err, "unable to stop Azure instance")
					}
				}
			}

			// Wait for all Azure instances to completely stop
			for _, vmName := range instanceNameList {
				log.Infof("[Wait]: Waiting for Azure instance '%v' to get in the stopped state", vmName)
				if err := azureStatus.WaitForAzureComputeDown(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					span.SetStatus(codes.Error, "failed to check instance poweroff status")
					span.RecordError(err)
					return stacktrace.Propagate(err, "instance poweroff status check failed")
				}
			}

			// Run probes during chaos
			if len(resultDetails.ProbeDetails) != 0 {
				if err = probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					span.SetStatus(codes.Error, "failed to run probes")
					span.RecordError(err)
					return stacktrace.Propagate(err, "failed to run probes")
				}
			}

			// Wait for Chaos interval
			log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)

			// Starting the Azure instance
			for _, vmName := range instanceNameList {
				log.Infof("[Chaos]: Starting back the Azure instance: %v", vmName)
				if experimentsDetails.ScaleSet == "enable" {
					if err := azureStatus.AzureScaleSetInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						span.SetStatus(codes.Error, "failed to start the Azure instance")
						span.RecordError(err)
						return stacktrace.Propagate(err, "unable to start the Azure instance")
					}
				} else {
					if err := azureStatus.AzureInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						span.SetStatus(codes.Error, "failed to start the Azure instance")
						span.RecordError(err)
						return stacktrace.Propagate(err, "unable to start the Azure instance")
					}
				}
			}

			// Wait for Azure instance to get in running state
			for _, vmName := range instanceNameList {
				log.Infof("[Wait]: Waiting for Azure instance '%v' to get in the running state", vmName)
				if err := azureStatus.WaitForAzureComputeUp(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					span.SetStatus(codes.Error, "failed to check instance power on status")
					span.RecordError(err)
					return stacktrace.Propagate(err, "instance power on status check failed")
				}
			}

			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}
	return nil
}

// watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, instanceNameList []string) {
	<-abort

	var instanceState string

	log.Info("[Abort]: Chaos Revert Started")
	for _, vmName := range instanceNameList {
		if experimentsDetails.ScaleSet == "enable" {
			scaleSetName, vmId := azureCommon.GetScaleSetNameAndInstanceId(vmName)
			instanceState, err = azureStatus.GetAzureScaleSetInstanceStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, scaleSetName, vmId)
		} else {
			instanceState, err = azureStatus.GetAzureInstanceStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName)
		}
		if err != nil {
			log.Errorf("[Abort]: Failed to get instance status when an abort signal is received: %v", err)
		}
		if instanceState != "VM running" && instanceState != "VM starting" {
			log.Info("[Abort]: Waiting for the Azure instance to get down")
			if err := azureStatus.WaitForAzureComputeDown(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
				log.Errorf("[Abort]: Instance power off status check failed: %v", err)
			}

			log.Info("[Abort]: Starting Azure instance as abort signal received")
			if experimentsDetails.ScaleSet == "enable" {
				if err := azureStatus.AzureScaleSetInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					log.Errorf("[Abort]: Unable to start the Azure instance: %v", err)
				}
			} else {
				if err := azureStatus.AzureInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					log.Errorf("[Abort]: Unable to start the Azure instance: %v", err)
				}
			}
		}

		log.Info("[Abort]: Waiting for the Azure instance to start")
		err := azureStatus.WaitForAzureComputeUp(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName)
		if err != nil {
			log.Errorf("[Abort]: Instance power on status check failed: %v", err)
			log.Errorf("[Abort]: Azure instance %v failed to start after an abort signal is received", vmName)
		}
	}
	log.Infof("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
