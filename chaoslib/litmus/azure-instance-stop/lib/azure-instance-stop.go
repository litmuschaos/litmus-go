package lib

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/instance-stop/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	azureCommon "github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"
	azureStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure/instance"
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

// PrepareAzureStop will initialize instanceNameList and start chaos injection based on sequence method selected
func PrepareAzureStop(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
	instanceNameList := strings.Split(experimentsDetails.AzureInstanceName, ",")
	if len(instanceNameList) == 0 {
		return errors.Errorf("no instance name found to stop")
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

// injectChaosInSerialMode will inject the azure instance termination in serial mode that is one after the other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceNameList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
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
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on azure instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			// PowerOff the instance serially
			for i, vmName := range instanceNameList {

				// Stopping the Azure instance
				log.Infof("[Chaos]: Stopping the Azure instance: %v", vmName)
				if experimentsDetails.ScaleSet == "enable" {
					if err := azureStatus.AzureScaleSetInstanceStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						return errors.Errorf("unable to stop the Azure instance, err: %v", err)
					}
				} else {
					if err := azureStatus.AzureInstanceStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						return errors.Errorf("unable to stop the Azure instance, err: %v", err)
					}
				}

				// Wait for Azure instance to completely stop
				log.Infof("[Wait]: Waiting for Azure instance '%v' to get in the stopped state", vmName)
				if err := azureStatus.WaitForAzureComputeDown(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					return errors.Errorf("instance poweroff status check failed, err: %v", err)
				}

				// Run the probes during chaos
				// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
				if len(resultDetails.ProbeDetails) != 0 && i == 0 {
					if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
						return err
					}
				}

				// Wait for Chaos interval
				log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
				common.WaitForDuration(experimentsDetails.ChaosInterval)

				// Starting the Azure instance
				log.Info("[Chaos]: Starting back the Azure instance")
				if experimentsDetails.ScaleSet == "enable" {
					if err := azureStatus.AzureScaleSetInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						return errors.Errorf("unable to start the Azure instance, err: %v", err)
					}
				} else {
					if err := azureStatus.AzureInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						return errors.Errorf("unable to start the Azure instance, err: %v", err)
					}
				}

				// Wait for Azure instance to get in running state
				log.Infof("[Wait]: Waiting for Azure instance '%v' to get in the running state", vmName)
				if err := azureStatus.WaitForAzureComputeUp(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					return errors.Errorf("instance power on status check failed, err: %v", err)
				}
			}
			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}
	return nil
}

// injectChaosInParallelMode will inject the azure instance termination in parallel mode that is all at once
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
				log.Infof("[Chaos]: Stopping the Azure instance: %v", vmName)
				if experimentsDetails.ScaleSet == "enable" {
					if err := azureStatus.AzureScaleSetInstanceStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						return errors.Errorf("unable to stop azure instance, err: %v", err)
					}
				} else {
					if err := azureStatus.AzureInstanceStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						return errors.Errorf("unable to stop azure instance, err: %v", err)
					}
				}
			}

			// Wait for all Azure instances to completely stop
			for _, vmName := range instanceNameList {
				log.Infof("[Wait]: Waiting for Azure instance '%v' to get in the stopped state", vmName)
				if err := azureStatus.WaitForAzureComputeDown(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					return errors.Errorf("instance poweroff status check failed, err: %v", err)
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

			// Starting the Azure instance
			for _, vmName := range instanceNameList {
				log.Infof("[Chaos]: Starting back the Azure instance: %v", vmName)
				if experimentsDetails.ScaleSet == "enable" {
					if err := azureStatus.AzureScaleSetInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						return errors.Errorf("unable to start the Azure instance, err: %v", err)
					}
				} else {
					if err := azureStatus.AzureInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
						return errors.Errorf("unable to start the Azure instance, err: %v", err)
					}
				}
			}

			// Wait for Azure instance to get in running state
			for _, vmName := range instanceNameList {
				log.Infof("[Wait]: Waiting for Azure instance '%v' to get in the running state", vmName)
				if err := azureStatus.WaitForAzureComputeUp(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					return errors.Errorf("instance power on status check failed, err: %v", err)
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
			log.Errorf("[Abort]: Fail to get instance status when an abort signal is received, err: %v", err)
		}
		if instanceState != "VM running" && instanceState != "VM starting" {
			log.Info("[Abort]: Waiting for the Azure instance to get down")
			if err := azureStatus.WaitForAzureComputeDown(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
				log.Errorf("[Abort]: Instance power off status check failed, err: %v", err)
			}

			log.Info("[Abort]: Starting Azure instance as abort signal received")
			if experimentsDetails.ScaleSet == "enable" {
				if err := azureStatus.AzureScaleSetInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					log.Errorf("[Abort]: Unable to start the Azure instance, err: %v", err)
				}
			} else {
				if err := azureStatus.AzureInstanceStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName); err != nil {
					log.Errorf("[Abort]: Unable to start the Azure instance, err: %v", err)
				}
			}
		}

		log.Info("[Abort]: Waiting for the Azure instance to start")
		err := azureStatus.WaitForAzureComputeUp(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ScaleSet, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, vmName)
		if err != nil {
			log.Errorf("[Abort]: Instance power on status check failed, err: %v", err)
			log.Errorf("[Abort]: Azure instance %v failed to start after an abort signal is received", vmName)
		}
	}
	log.Infof("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
