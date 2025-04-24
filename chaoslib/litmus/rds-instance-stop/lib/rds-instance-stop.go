package lib

import (
	"fmt"
	"go.opentelemetry.io/otel"
	"golang.org/x/net/context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	awslib "github.com/litmuschaos/litmus-go/pkg/cloud/aws/rds"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/rds-instance-stop/types"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/palantir/stacktrace"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
)

var (
	err           error
	inject, abort chan os.Signal
)

func PrepareRDSInstanceStop(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PrepareRDSInstanceStop")
	defer span.End()

	// Inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// Abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	// Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// Get the instance identifier or list of instance identifiers
	instanceIdentifierList := strings.Split(experimentsDetails.RDSInstanceIdentifier, ",")
	if experimentsDetails.RDSInstanceIdentifier == "" || len(instanceIdentifierList) == 0 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "no RDS instance identifier found to stop"}
	}

	instanceIdentifierList = common.FilterBasedOnPercentage(experimentsDetails.InstanceAffectedPerc, instanceIdentifierList)
	log.Infof("[Chaos]:Number of Instance targeted: %v", len(instanceIdentifierList))

	// Watching for the abort signal and revert the chaos
	go abortWatcher(experimentsDetails, instanceIdentifierList, chaosDetails)

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(ctx, experimentsDetails, instanceIdentifierList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err = injectChaosInParallelMode(ctx, experimentsDetails, instanceIdentifierList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
	}

	// Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// injectChaosInSerialMode will inject the rds instance state in serial mode that is one after other
func injectChaosInSerialMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, instanceIdentifierList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// Stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:
		// ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

			log.Infof("[Info]: Target instance identifier list, %v", instanceIdentifierList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on rds instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			for i, identifier := range instanceIdentifierList {

				// Stopping the RDS instance
				log.Info("[Chaos]: Stopping the desired RDS instance")
				if err := awslib.RDSInstanceStop(identifier, experimentsDetails.Region); err != nil {
					return stacktrace.Propagate(err, "rds instance failed to stop")
				}

				common.SetTargets(identifier, "injected", "RDS", chaosDetails)

				// Wait for rds instance to completely stop
				log.Infof("[Wait]: Wait for RDS instance '%v' to get in stopped state", identifier)
				if err := awslib.WaitForRDSInstanceDown(experimentsDetails.Timeout, experimentsDetails.Delay, identifier, experimentsDetails.Region); err != nil {
					return stacktrace.Propagate(err, "rds instance failed to stop")
				}

				// Run the probes during chaos
				// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
				if len(resultDetails.ProbeDetails) != 0 && i == 0 {
					if err = probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
						return stacktrace.Propagate(err, "failed to run probes")
					}
				}

				// Wait for chaos interval
				log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
				time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)

				// Starting the RDS instance
				log.Info("[Chaos]: Starting back the RDS instance")
				if err = awslib.RDSInstanceStart(identifier, experimentsDetails.Region); err != nil {
					return stacktrace.Propagate(err, "rds instance failed to start")
				}

				// Wait for rds instance to get in available state
				log.Infof("[Wait]: Wait for RDS instance '%v' to get in available state", identifier)
				if err := awslib.WaitForRDSInstanceUp(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.Region, identifier); err != nil {
					return stacktrace.Propagate(err, "rds instance failed to start")
				}

				common.SetTargets(identifier, "reverted", "RDS", chaosDetails)
			}
			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}
	return nil
}

// injectChaosInParallelMode will inject the rds instance termination in parallel mode that is all at once
func injectChaosInParallelMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, instanceIdentifierList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

			log.Infof("[Info]: Target instance identifier list, %v", instanceIdentifierList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on rds instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			// PowerOff the instance
			for _, identifier := range instanceIdentifierList {
				// Stopping the RDS instance
				log.Info("[Chaos]: Stopping the desired RDS instance")
				if err := awslib.RDSInstanceStop(identifier, experimentsDetails.Region); err != nil {
					return stacktrace.Propagate(err, "rds instance failed to stop")
				}
				common.SetTargets(identifier, "injected", "RDS", chaosDetails)
			}

			for _, identifier := range instanceIdentifierList {
				// Wait for rds instance to completely stop
				log.Infof("[Wait]: Wait for RDS instance '%v' to get in stopped state", identifier)
				if err := awslib.WaitForRDSInstanceDown(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.Region, identifier); err != nil {
					return stacktrace.Propagate(err, "rds instance failed to stop")
				}
				common.SetTargets(identifier, "reverted", "RDS", chaosDetails)
			}

			// Run the probes during chaos
			if len(resultDetails.ProbeDetails) != 0 {
				if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return stacktrace.Propagate(err, "failed to run probes")
				}
			}

			// Wait for chaos interval
			log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
			time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)

			// Starting the RDS instance
			for _, identifier := range instanceIdentifierList {
				log.Info("[Chaos]: Starting back the RDS instance")
				if err = awslib.RDSInstanceStart(identifier, experimentsDetails.Region); err != nil {
					return stacktrace.Propagate(err, "rds instance failed to start")
				}
			}

			for _, identifier := range instanceIdentifierList {
				// Wait for rds instance to get in available state
				log.Infof("[Wait]: Wait for RDS instance '%v' to get in available state", identifier)
				if err := awslib.WaitForRDSInstanceUp(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.Region, identifier); err != nil {
					return stacktrace.Propagate(err, "rds instance failed to start")
				}
			}

			for _, identifier := range instanceIdentifierList {
				common.SetTargets(identifier, "reverted", "RDS", chaosDetails)
			}
			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}
	return nil
}

// watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, instanceIdentifierList []string, chaosDetails *types.ChaosDetails) {

	<-abort

	log.Info("[Abort]: Chaos Revert Started")
	for _, identifier := range instanceIdentifierList {
		instanceState, err := awslib.GetRDSInstanceStatus(identifier, experimentsDetails.Region)
		if err != nil {
			log.Errorf("Failed to get instance status when an abort signal is received: %v", err)
		}
		if instanceState != "running" {

			log.Info("[Abort]: Waiting for the RDS instance to get down")
			if err := awslib.WaitForRDSInstanceDown(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.Region, identifier); err != nil {
				log.Errorf("Unable to wait till stop of the instance: %v", err)
			}

			log.Info("[Abort]: Starting RDS instance as abort signal received")
			err := awslib.RDSInstanceStart(identifier, experimentsDetails.Region)
			if err != nil {
				log.Errorf("RDS instance failed to start when an abort signal is received: %v", err)
			}
		}
		common.SetTargets(identifier, "reverted", "RDS", chaosDetails)
	}
	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
