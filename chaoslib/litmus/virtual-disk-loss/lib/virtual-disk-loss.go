package lib

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/virtual-disk-loss/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	azureStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure"
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

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:

		//get the volume name  or list of volume names
		diskNameList := strings.Split(experimentsDetails.VirtualDiskName, ",")
		if len(diskNameList) == 0 {
			return errors.Errorf("no volume names found to detach")
		}
		diskList, err := azureStatus.GetInstanceDiskList(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName)
		if err != nil {
			log.Errorf("err: %v", err)
			return errors.Errorf("error fetching virtual disks")
		}
		// watching for the abort signal and revert the chaos
		go abortWatcher(experimentsDetails, diskList, diskNameList)

		switch strings.ToLower(experimentsDetails.Sequence) {
		case "serial":
			if err = InjectChaosInSerialMode(experimentsDetails, diskNameList, diskList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return err
			}
		case "parallel":
			if err = InjectChaosInParallelMode(experimentsDetails, diskNameList, diskList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
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

func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, diskNameList []string, diskList *[]compute.DataDisk, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
		log.Info("[Chaos]: Detaching the Virtual disks from the instance")
		if err = azureStatus.DetachDiskMultiple(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName, diskNameList); err != nil {
			return errors.Errorf("failed to detach disks, err: %v", err)
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
			common.SetTargets(diskName, "reverted", "VirtualDisk", chaosDetails)
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, diskNameList []string, diskList *[]compute.DataDisk, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
			log.Infof("[Chaos]: Detaching the Virtual %v from the instance", diskName)
			if err = azureStatus.DetachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName, diskName); err != nil {
				return errors.Errorf("failed to detach disks, err: %v", err)
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

			//Getting the virtual disk status
			diskState, err := azureStatus.GetDiskStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, diskName)
			if err != nil {
				return errors.Errorf("failed to get the virtual disk status, err: %v", err)
			}

			switch diskState {
			case "Attached":
				log.Info("[Skip]: The Virtual Disk is already attached")
			case "Unattached":
				//Attaching the virtual disks to the instance
				log.Info("[Chaos]: Attaching the Virtual disk back to the instance")
				if err = azureStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName, diskList); err != nil {
					return errors.Errorf("disk attachment failed, err: %v", err)
				}
			default:
				return errors.Errorf("Cannot attach disk, this is because the disk is either currently being created or the VM is not running, current disk state: %v", diskState)
			}
			common.SetTargets(diskName, "reverted", "VirtualDisk", chaosDetails)
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

// watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, diskList *[]compute.DataDisk, diskNameList []string) {
	<-abort

	log.Info("[Abort]: Chaos Revert Started")

	// Checking if all disk detached properly
	log.Info("[Abort]: Waiting for disk(s) to detach properly")
	for _, diskName := range diskNameList {
		//Getting the virtual disk status
		retry.
			Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
			Wait(time.Duration(experimentsDetails.Delay) * time.Second).
			Try(func(attempt uint) error {
				diskState, err := azureStatus.GetDiskStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, diskName)
				if err != nil {
					errors.Errorf("failed to get the disk status, err: %v", err)
				}
				if diskState != "Unattached" {
					log.Infof("[Abort]: Disk is not yet detached properly, state: %v", diskState)
					return errors.Errorf("Disk is not yet detached properly, state: %v", diskState)
				}
				log.Infof("[Abort]: Disk %v state: %v", diskName, diskState)
				return nil
			})
	}
	log.Info("[Abort]: Disk(s) detached properly")

	log.Info("[Abort]: Attaching disk(s) as abort signal recieved")
	if err := azureStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName, diskList); err != nil {
		log.Errorf("unable to attach disk, err: %v", err)
	}
	log.Infof("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
