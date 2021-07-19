package lib

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/disk-loss/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	azureStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure/disk"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
)

var (
	err           error
	inject, abort chan os.Signal
)

//PrepareChaos contains the prepration and injection steps for the experiment
func PrepareChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//get the volume name  or list of volume names
	diskNameList := strings.Split(experimentsDetails.VirtualDiskNames, ",")
	if len(diskNameList) == 0 {
		return errors.Errorf("no volume names found to detach")
	}
	attachedDisks, err := azureStatus.GetInstanceDiskList(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName)
	if err != nil {
		log.Errorf("err: %v", err)
		return errors.Errorf("error fetching virtual disks")
	}

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:

		// watching for the abort signal and revert the chaos
		go abortWatcher(experimentsDetails, attachedDisks, diskNameList, chaosDetails)

		switch strings.ToLower(experimentsDetails.Sequence) {
		case "serial":
			if err = injectChaosInSerialMode(experimentsDetails, diskNameList, attachedDisks, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return err
			}
		case "parallel":
			if err = injectChaosInParallelMode(experimentsDetails, diskNameList, attachedDisks, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return err
			}
		default:
			return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
		}

		//Waiting for the ramp time after chaos injection
		if experimentsDetails.RampTime != 0 {
			log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
			common.WaitForDuration(experimentsDetails.RampTime)
		}
	}
	return nil
}

// injectChaosInParallelMode will inject the azure disk loss chaos in parallel mode that is all at once
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, diskNameList []string, diskList *[]compute.DataDisk, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on ec2 instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		// Detaching the virtual disks
		log.Info("[Chaos]: Detaching the virtual disks from the instance")
		if err = azureStatus.DetachMultipleDisks(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName, diskNameList); err != nil {
			return errors.Errorf("failed to detach disks, err: %v", err)
		}

		for _, diskName := range diskNameList {
			log.Infof("[Wait]: Waiting for Disk '%v' to detach", diskName)
			if err := azureStatus.WaitForDiskToDetach(experimentsDetails, diskName); err != nil {
				return errors.Errorf("disk attach check failed, err: %v", err)
			}
		}

		for _, diskName := range diskNameList {
			common.SetTargets(diskName, "injected", "VirtualDisk", chaosDetails)
		}

		// run the probes during chaos
		if len(resultDetails.ProbeDetails) != 0 {
			if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
				return err
			}
		}

		//Wait for chaos duration
		log.Infof("[Wait]: Waiting for the chaos interval of %vs", experimentsDetails.ChaosInterval)
		common.WaitForDuration(experimentsDetails.ChaosInterval)

		//Attaching the virtual disks to the instance
		log.Info("[Chaos]: Attaching the Virtual disks back to the instance")
		if err = azureStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName, diskList); err != nil {
			return errors.Errorf("virtual disk attachment failed, err: %v", err)
		}

		for _, diskName := range diskNameList {
			log.Infof("[Wait]: Waiting for Disk '%v' to attach", diskName)
			if err := azureStatus.WaitForDiskToAttach(experimentsDetails, diskName); err != nil {
				return errors.Errorf("disk attach check failed, err: %v", err)
			}
		}

		for _, diskName := range diskNameList {
			common.SetTargets(diskName, "reverted", "VirtualDisk", chaosDetails)
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

//injectChaosInSerialMode will inject the azure disk loss chaos in serial mode that is one after other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, diskNameList []string, diskList *[]compute.DataDisk, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on ec2 instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		for i, diskName := range diskNameList {
			// Detaching the virtual disks
			log.Infof("[Chaos]: Detaching %v from the instance", diskName)
			if err = azureStatus.DetachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName, diskName); err != nil {
				return errors.Errorf("failed to detach disks, err: %v", err)
			}

			log.Infof("[Wait]: Waiting for Disk '%v' to detach", diskName)
			if err := azureStatus.WaitForDiskToDetach(experimentsDetails, diskName); err != nil {
				return errors.Errorf("disk detach check failed, err: %v", err)
			}

			common.SetTargets(diskName, "injected", "VirtualDisk", chaosDetails)

			// run the probes during chaos
			if len(resultDetails.ProbeDetails) != 0 && i == 0 {
				if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return err
				}
			}

			//Wait for chaos duration
			log.Infof("[Wait]: Waiting for the chaos interval of %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)

			//Attaching the virtual disks to the instance
			log.Infof("[Chaos]: Attaching %v back to the instance", diskName)
			if err = azureStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName, diskList); err != nil {
				return errors.Errorf("disk attachment failed, err: %v", err)
			}

			log.Infof("[Wait]: Waiting for Disk '%v' to attach", diskName)
			if err := azureStatus.WaitForDiskToAttach(experimentsDetails, diskName); err != nil {
				return errors.Errorf("disk attach check failed, err: %v", err)
			}

			common.SetTargets(diskName, "reverted", "VirtualDisk", chaosDetails)
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

// abortWatcher will be watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, diskList *[]compute.DataDisk, diskNameList []string, chaosDetails *types.ChaosDetails) {
	<-abort

	log.Info("[Abort]: Chaos Revert Started")

	// Attaching disks back to the instance
	// log.Info("[Abort]: Waiting for disk(s) to detach properly")
	// for _, diskName := range diskNameList {
	// 	diskState, err := azureStatus.GetDiskStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, diskName)
	// 	if err != nil {
	// 		errors.Errorf("failed to get the disk status, err: %v", err)
	// 	}
	// 	if diskState != "Attached" {

	// 		for _, diskName := range diskNameList {
	// 			azureStatus.WaitForDiskToDetach(experimentsDetails, diskName)
	// 		}
	// 	}
	// }
	// log.Info("[Abort]: Disk(s) detached properly")

	log.Info("[Abort]: Attaching disk(s) as abort signal recieved")

	retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			if err := azureStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName, diskList); err != nil {
				log.Infof("Waiting for detaching disks")
				return errors.Errorf("Waiting for detaching disks, err: %v", err)
			}
			return nil
		})

	for _, diskName := range diskNameList {
		common.SetTargets(diskName, "reverted", "VirtualDisk", chaosDetails)
	}

	log.Infof("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
