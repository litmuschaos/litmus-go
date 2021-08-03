package lib

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/run-command/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	azureStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure/run-command"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

var (
	err           error
	inject, abort chan os.Signal
)

func PrepareAzureRunCommandChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
	if len(instanceNameList) == 0 {
		return errors.Errorf("no instance name found")
	}

	// watching for the abort signal and revert the chaos
	go abortWatcher(experimentsDetails, instanceNameList)

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(experimentsDetails, instanceNameList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, instanceNameList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
	}

	// Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceNameList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
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

			// PowerOff the instances parallely
			for _, vmName := range instanceNameList {
				// Stopping the Azure instance
				log.Infof("[Chaos]: Running script on the Azure instance: %v", vmName)
				if err := azureStatus.PerformRunCommand(experimentsDetails, vmName); err != nil {
					return errors.Errorf("unable to run script on azure instance, err: %v", err)
				}
			}

			// Run probes during chaos
			if len(resultDetails.ProbeDetails) != 0 {
				if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return err
				}
			}

			// Wait for Chaos interval
			log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)

			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}
	return nil
}

func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceNameList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	return nil
}

func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, instanceNameList []string) {

}
