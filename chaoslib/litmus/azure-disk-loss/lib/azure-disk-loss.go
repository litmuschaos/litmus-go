package lib

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/disk-loss/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	diskStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure/disk"
	instanceStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure/instance"
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

	//get the disk name  or list of disk names
	diskNameList := strings.Split(experimentsDetails.VirtualDiskNames, ",")
	if len(diskNameList) == 0 {
		return errors.Errorf("no volume names found to detach")
	}
	instanceNamesWithDiskNames, err := diskStatus.GetInstanceNameForDisks(diskNameList, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup)

	if err != nil {
		return errors.Errorf("error fetching attached instances for disks, err: %v", err)
	}

	// Get the instance name with attached disks
	attachedDisksWithInstance := make(map[string]*[]compute.DataDisk)

	for instanceName := range instanceNamesWithDiskNames {
		attachedDisksWithInstance[instanceName], err = diskStatus.GetInstanceDiskList(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.ScaleSet, instanceName)
		if err != nil {
			return errors.Errorf("error fetching virtual disks, err: %v", err)
		}
	}

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:

		// watching for the abort signal and revert the chaos
		go abortWatcher(experimentsDetails, attachedDisksWithInstance, instanceNamesWithDiskNames, chaosDetails)

		switch strings.ToLower(experimentsDetails.Sequence) {
		case "serial":
			if err = injectChaosInSerialMode(experimentsDetails, instanceNamesWithDiskNames, attachedDisksWithInstance, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return err
			}
		case "parallel":
			if err = injectChaosInParallelMode(experimentsDetails, instanceNamesWithDiskNames, attachedDisksWithInstance, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
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
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceNamesWithDiskNames map[string][]string, attachedDisksWithInstance map[string]*[]compute.DataDisk, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on azure virtual disk"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		// Detaching the virtual disks
		log.Info("[Chaos]: Detaching the virtual disks from the instances")
		for instanceName, diskNameList := range instanceNamesWithDiskNames {
			if err = diskStatus.DetachDisks(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet, diskNameList); err != nil {
				return errors.Errorf("failed to detach disks, err: %v", err)
			}
		}
		// Waiting for disk to be detached
		for _, diskNameList := range instanceNamesWithDiskNames {
			for _, diskName := range diskNameList {
				log.Infof("[Wait]: Waiting for Disk '%v' to detach", diskName)
				if err := diskStatus.WaitForDiskToDetach(experimentsDetails, diskName); err != nil {
					return errors.Errorf("disk attach check failed, err: %v", err)
				}
			}
		}

		// Updating the result details
		for _, diskNameList := range instanceNamesWithDiskNames {
			for _, diskName := range diskNameList {
				common.SetTargets(diskName, "detached", "VirtualDisk", chaosDetails)
			}
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
		log.Info("[Chaos]: Attaching the Virtual disks back to the instances")
		for instanceName, diskNameList := range attachedDisksWithInstance {
			if err = diskStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet, diskNameList); err != nil {
				return errors.Errorf("virtual disk attachment failed, err: %v", err)
			}
		}

		// Wait for disk to be attached
		for _, diskNameList := range instanceNamesWithDiskNames {
			for _, diskName := range diskNameList {
				log.Infof("[Wait]: Waiting for Disk '%v' to attach", diskName)
				if err := diskStatus.WaitForDiskToAttach(experimentsDetails, diskName); err != nil {
					return errors.Errorf("disk attach check failed, err: %v", err)
				}
			}
		}

		// Updating the result details
		for _, diskNameList := range instanceNamesWithDiskNames {
			for _, diskName := range diskNameList {
				common.SetTargets(diskName, "re-attached", "VirtualDisk", chaosDetails)
			}
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

//injectChaosInSerialMode will inject the azure disk loss chaos in serial mode that is one after other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceNamesWithDiskNames map[string][]string, attachedDisksWithInstance map[string]*[]compute.DataDisk, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on azure virtual disks"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		for instanceName, diskNameList := range instanceNamesWithDiskNames {
			for i, diskName := range diskNameList {
				// Converting diskName to list type because DetachDisks() accepts a list type
				diskNameToList := []string{diskName}

				// Detaching the virtual disks
				log.Infof("[Chaos]: Detaching %v from the instance", diskName)
				if err = diskStatus.DetachDisks(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet, diskNameToList); err != nil {
					return errors.Errorf("failed to detach disks, err: %v", err)
				}

				// Waiting for disk to be detached
				log.Infof("[Wait]: Waiting for Disk '%v' to detach", diskName)
				if err := diskStatus.WaitForDiskToDetach(experimentsDetails, diskName); err != nil {
					return errors.Errorf("disk detach check failed, err: %v", err)
				}

				common.SetTargets(diskName, "detached", "VirtualDisk", chaosDetails)

				// run the probes during chaos
				// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
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
				if err = diskStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet, attachedDisksWithInstance[instanceName]); err != nil {
					return errors.Errorf("disk attachment failed, err: %v", err)
				}

				// Waiting for disk to be attached
				log.Infof("[Wait]: Waiting for Disk '%v' to attach", diskName)
				if err := diskStatus.WaitForDiskToAttach(experimentsDetails, diskName); err != nil {
					return errors.Errorf("disk attach check failed, err: %v", err)
				}

				common.SetTargets(diskName, "re-attached", "VirtualDisk", chaosDetails)
			}
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

// abortWatcher will be watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, attachedDisksWithInstance map[string]*[]compute.DataDisk, instanceNamesWithDiskNames map[string][]string, chaosDetails *types.ChaosDetails) {
	<-abort

	log.Info("[Abort]: Chaos Revert Started")

	log.Info("[Abort]: Attaching disk(s) as abort signal recieved")

	for instanceName, diskList := range attachedDisksWithInstance {
		// Checking for provisioning state of the vm instances
		err = retry.
			Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
			Wait(time.Duration(experimentsDetails.Delay) * time.Second).
			Try(func(attempt uint) error {
				status, err := instanceStatus.GetAzureInstanceProvisionStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet)
				if err != nil {
					return errors.Errorf("Failed to get instance, err: %v", err)
				}
				if status != "Provisioning succeeded" {
					return errors.Errorf("instance is updating, waiting for instance to finish update")
				}
				return nil
			})
		if err != nil {
			log.Errorf("[Error]: Instance is still in 'updating' state after timeout, re-attach might fail")
		}
		log.Infof("[Abort]: Attaching disk(s) to instance: %v", instanceName)
		for _, disk := range *diskList {
			diskStatusString, err := diskStatus.GetDiskStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, *disk.Name)
			if err != nil {
				log.Errorf("Failed to get disk status, err: %v", err)
			}
			if diskStatusString != "Attached" {
				if err := diskStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet, diskList); err != nil {
					log.Errorf("failed to attach disk '%v, manual revert required, err: %v", err)
				} else {
					common.SetTargets(*disk.Name, "re-attached", "VirtualDisk", chaosDetails)
				}
			}
		}
	}

	log.Infof("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
