package lib

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/cloud/gcp"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-disk-loss/types"
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

func PrepareDiskVolumeLossByLabel(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var deviceNamesList []string

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

	diskVolumeNamesList := common.FilterBasedOnPercentage(experimentsDetails.DiskAffectedPerc, experimentsDetails.TargetDiskVolumeNamesList)

	for i := range diskVolumeNamesList {

		instanceName, err := gcp.GetVolumeAttachmentDetails(experimentsDetails.GCPProjectID, experimentsDetails.DiskZones, diskVolumeNamesList[i])
		if err != nil || instanceName == "" {
			return errors.Errorf("failed to get the attachment info, err: %v", err)
		}

		deviceName, err := gcp.GetDiskDeviceNameForVM(diskVolumeNamesList[i], experimentsDetails.GCPProjectID, experimentsDetails.DiskZones, instanceName)
		if err != nil {
			return err
		}

		experimentsDetails.TargetDiskInstanceNamesList = append(experimentsDetails.TargetDiskInstanceNamesList, instanceName)
		deviceNamesList = append(deviceNamesList, deviceName)
	}

	select {

	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)

	default:
		// watching for the abort signal and revert the chaos
		go abortWatcher(experimentsDetails, diskVolumeNamesList, deviceNamesList, experimentsDetails.TargetDiskInstanceNamesList, experimentsDetails.DiskZones, abort, chaosDetails)

		switch strings.ToLower(experimentsDetails.Sequence) {
		case "serial":
			if err = injectChaosInSerialMode(experimentsDetails, diskVolumeNamesList, deviceNamesList, experimentsDetails.TargetDiskInstanceNamesList, experimentsDetails.DiskZones, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return err
			}
		case "parallel":
			if err = injectChaosInParallelMode(experimentsDetails, diskVolumeNamesList, deviceNamesList, experimentsDetails.TargetDiskInstanceNamesList, experimentsDetails.DiskZones, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return err
			}
		default:
			return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
		}
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	return nil
}

//injectChaosInSerialMode will inject the disk loss chaos in serial mode which means one after the other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetDiskVolumeNamesList, deviceNamesList, instanceNamesList []string, zone string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on VM instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		for i := range targetDiskVolumeNamesList {

			//Detaching the disk volume from the instance
			log.Info("[Chaos]: Detaching the disk volume from the instance")
			if err = gcp.DiskVolumeDetach(instanceNamesList[i], experimentsDetails.GCPProjectID, zone, deviceNamesList[i]); err != nil {
				return errors.Errorf("disk detachment failed, err: %v", err)
			}

			common.SetTargets(targetDiskVolumeNamesList[i], "injected", "DiskVolume", chaosDetails)

			//Wait for disk volume detachment
			log.Infof("[Wait]: Wait for disk volume detachment for volume %v", targetDiskVolumeNamesList[i])
			if err = gcp.WaitForVolumeDetachment(targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				return errors.Errorf("unable to detach the disk volume from the vm instance, err: %v", err)
			}

			// run the probes during chaos
			// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
			if len(resultDetails.ProbeDetails) != 0 && i == 0 {
				if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return err
				}
			}

			//Wait for chaos duration
			log.Infof("[Wait]: Waiting for the chaos interval of %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)

			//Getting the disk volume attachment status
			diskState, err := gcp.GetDiskVolumeState(targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone)
			if err != nil {
				return errors.Errorf("failed to get the disk volume status, err: %v", err)
			}

			switch diskState {
			case "attached":
				log.Info("[Skip]: The disk volume is already attached")
			default:
				//Attaching the disk volume to the instance
				log.Info("[Chaos]: Attaching the disk volume back to the instance")
				if err = gcp.DiskVolumeAttach(instanceNamesList[i], experimentsDetails.GCPProjectID, zone, deviceNamesList[i], targetDiskVolumeNamesList[i]); err != nil {
					return errors.Errorf("disk attachment failed, err: %v", err)
				}

				//Wait for disk volume attachment
				log.Infof("[Wait]: Wait for disk volume attachment for %v volume", targetDiskVolumeNamesList[i])
				if err = gcp.WaitForVolumeAttachment(targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
					return errors.Errorf("unable to attach the disk volume to the vm instance, err: %v", err)
				}
			}

			common.SetTargets(targetDiskVolumeNamesList[i], "reverted", "DiskVolume", chaosDetails)
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	return nil
}

//injectChaosInParallelMode will inject the disk loss chaos in parallel mode that means all at once
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetDiskVolumeNamesList, deviceNamesList, instanceNamesList []string, zone string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on vm instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		for i := range targetDiskVolumeNamesList {

			//Detaching the disk volume from the instance
			log.Info("[Chaos]: Detaching the disk volume from the instance")
			if err = gcp.DiskVolumeDetach(instanceNamesList[i], experimentsDetails.GCPProjectID, zone, deviceNamesList[i]); err != nil {
				return errors.Errorf("disk detachment failed, err: %v", err)
			}

			common.SetTargets(targetDiskVolumeNamesList[i], "injected", "DiskVolume", chaosDetails)
		}

		for i := range targetDiskVolumeNamesList {

			//Wait for disk volume detachment
			log.Infof("[Wait]: Wait for disk volume detachment for volume %v", targetDiskVolumeNamesList[i])
			if err = gcp.WaitForVolumeDetachment(targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				return errors.Errorf("unable to detach the disk volume from the vm instance, err: %v", err)
			}
		}

		// run the probes during chaos
		if len(resultDetails.ProbeDetails) != 0 {
			if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
				return err
			}
		}

		//Wait for chaos interval
		log.Infof("[Wait]: Waiting for the chaos interval of %vs", experimentsDetails.ChaosInterval)
		common.WaitForDuration(experimentsDetails.ChaosInterval)

		for i := range targetDiskVolumeNamesList {

			//Getting the disk volume attachment status
			diskState, err := gcp.GetDiskVolumeState(targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone)
			if err != nil {
				return errors.Errorf("failed to get the disk status, err: %v", err)
			}

			switch diskState {
			case "attached":
				log.Info("[Skip]: The disk volume is already attached")
			default:
				//Attaching the disk volume to the instance
				log.Info("[Chaos]: Attaching the disk volume to the instance")
				if err = gcp.DiskVolumeAttach(instanceNamesList[i], experimentsDetails.GCPProjectID, zone, deviceNamesList[i], targetDiskVolumeNamesList[i]); err != nil {
					return errors.Errorf("disk attachment failed, err: %v", err)
				}

				//Wait for disk volume attachment
				log.Infof("[Wait]: Wait for disk volume attachment for volume %v", targetDiskVolumeNamesList[i])
				if err = gcp.WaitForVolumeAttachment(targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
					return errors.Errorf("unable to attach the disk volume to the vm instance, err: %v", err)
				}
			}

			common.SetTargets(targetDiskVolumeNamesList[i], "reverted", "DiskVolume", chaosDetails)
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	return nil
}

// AbortWatcher will watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, targetDiskVolumeNamesList, deviceNamesList, instanceNamesList []string, zone string, abort chan os.Signal, chaosDetails *types.ChaosDetails) {

	<-abort

	log.Info("[Abort]: Chaos Revert Started")

	for i := range targetDiskVolumeNamesList {

		//Getting the disk volume attachment status
		diskState, err := gcp.GetDiskVolumeState(targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone)
		if err != nil {
			log.Errorf("failed to get the disk state when an abort signal is received, err: %v", err)
		}

		if diskState != "attached" {

			//Wait for disk volume detachment
			//We first wait for the volume to get in detached state then we are attaching it.
			log.Info("[Abort]: Wait for complete disk volume detachment")

			if err = gcp.WaitForVolumeDetachment(targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				log.Errorf("unable to detach the disk volume, err: %v", err)
			}

			//Attaching the disk volume from the instance
			log.Info("[Chaos]: Attaching the disk volume from the instance")

			err = gcp.DiskVolumeAttach(instanceNamesList[i], experimentsDetails.GCPProjectID, zone, deviceNamesList[i], targetDiskVolumeNamesList[i])
			if err != nil {
				log.Errorf("disk attachment failed when an abort signal is received, err: %v", err)
			}
		}

		common.SetTargets(targetDiskVolumeNamesList[i], "reverted", "DiskVolume", chaosDetails)
	}

	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
