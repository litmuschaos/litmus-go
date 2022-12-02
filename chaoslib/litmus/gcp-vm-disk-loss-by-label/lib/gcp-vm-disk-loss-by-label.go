package lib

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/cloud/gcp"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-disk-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/palantir/stacktrace"
	"google.golang.org/api/compute/v1"
)

var (
	err           error
	inject, abort chan os.Signal
)

// PrepareDiskVolumeLossByLabel contains the prepration and injection steps for the experiment
func PrepareDiskVolumeLossByLabel(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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

	if err := getDeviceNamesAndVMInstanceNames(diskVolumeNamesList, computeService, experimentsDetails); err != nil {
		return err
	}

	select {

	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)

	default:
		// watching for the abort signal and revert the chaos
		go abortWatcher(computeService, experimentsDetails, diskVolumeNamesList, experimentsDetails.TargetDiskInstanceNamesList, experimentsDetails.Zones, abort, chaosDetails)

		switch strings.ToLower(experimentsDetails.Sequence) {
		case "serial":
			if err = injectChaosInSerialMode(computeService, experimentsDetails, diskVolumeNamesList, experimentsDetails.TargetDiskInstanceNamesList, experimentsDetails.Zones, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return stacktrace.Propagate(err, "could not run chaos in serial mode")
			}
		case "parallel":
			if err = injectChaosInParallelMode(computeService, experimentsDetails, diskVolumeNamesList, experimentsDetails.TargetDiskInstanceNamesList, experimentsDetails.Zones, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return stacktrace.Propagate(err, "could not run chaos in parallel mode")
			}
		default:
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
		}
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	return nil
}

// injectChaosInSerialMode will inject the disk loss chaos in serial mode which means one after the other
func injectChaosInSerialMode(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, targetDiskVolumeNamesList, instanceNamesList []string, zone string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
			if err = gcp.DiskVolumeDetach(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, zone, experimentsDetails.DeviceNamesList[i]); err != nil {
				return stacktrace.Propagate(err, "disk detachment failed")
			}

			common.SetTargets(targetDiskVolumeNamesList[i], "injected", "DiskVolume", chaosDetails)

			//Wait for disk volume detachment
			log.Infof("[Wait]: Wait for disk volume detachment for volume %v", targetDiskVolumeNamesList[i])
			if err = gcp.WaitForVolumeDetachment(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				return stacktrace.Propagate(err, "unable to detach the disk volume from the vm instance")
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
			diskState, err := gcp.GetDiskVolumeState(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the disk volume status")
			}

			switch diskState {
			case "attached":
				log.Info("[Skip]: The disk volume is already attached")
			default:
				//Attaching the disk volume to the instance
				log.Info("[Chaos]: Attaching the disk volume back to the instance")
				if err = gcp.DiskVolumeAttach(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, zone, experimentsDetails.DeviceNamesList[i], targetDiskVolumeNamesList[i]); err != nil {
					return stacktrace.Propagate(err, "disk attachment failed")
				}

				//Wait for disk volume attachment
				log.Infof("[Wait]: Wait for disk volume attachment for %v volume", targetDiskVolumeNamesList[i])
				if err = gcp.WaitForVolumeAttachment(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
					return stacktrace.Propagate(err, "unable to attach the disk volume to the vm instance")
				}
			}

			common.SetTargets(targetDiskVolumeNamesList[i], "reverted", "DiskVolume", chaosDetails)
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	return nil
}

// injectChaosInParallelMode will inject the disk loss chaos in parallel mode that means all at once
func injectChaosInParallelMode(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, targetDiskVolumeNamesList, instanceNamesList []string, zone string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
			if err = gcp.DiskVolumeDetach(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, zone, experimentsDetails.DeviceNamesList[i]); err != nil {
				return stacktrace.Propagate(err, "disk detachment failed")
			}

			common.SetTargets(targetDiskVolumeNamesList[i], "injected", "DiskVolume", chaosDetails)
		}

		for i := range targetDiskVolumeNamesList {

			//Wait for disk volume detachment
			log.Infof("[Wait]: Wait for disk volume detachment for volume %v", targetDiskVolumeNamesList[i])
			if err = gcp.WaitForVolumeDetachment(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				return stacktrace.Propagate(err, "unable to detach the disk volume from the vm instance")
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
			diskState, err := gcp.GetDiskVolumeState(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the disk status")
			}

			switch diskState {
			case "attached":
				log.Info("[Skip]: The disk volume is already attached")
			default:
				//Attaching the disk volume to the instance
				log.Info("[Chaos]: Attaching the disk volume to the instance")
				if err = gcp.DiskVolumeAttach(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, zone, experimentsDetails.DeviceNamesList[i], targetDiskVolumeNamesList[i]); err != nil {
					return stacktrace.Propagate(err, "disk attachment failed")
				}

				//Wait for disk volume attachment
				log.Infof("[Wait]: Wait for disk volume attachment for volume %v", targetDiskVolumeNamesList[i])
				if err = gcp.WaitForVolumeAttachment(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
					return stacktrace.Propagate(err, "unable to attach the disk volume to the vm instance")
				}
			}

			common.SetTargets(targetDiskVolumeNamesList[i], "reverted", "DiskVolume", chaosDetails)
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	return nil
}

// AbortWatcher will watching for the abort signal and revert the chaos
func abortWatcher(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, targetDiskVolumeNamesList, instanceNamesList []string, zone string, abort chan os.Signal, chaosDetails *types.ChaosDetails) {

	<-abort

	log.Info("[Abort]: Chaos Revert Started")

	for i := range targetDiskVolumeNamesList {

		//Getting the disk volume attachment status
		diskState, err := gcp.GetDiskVolumeState(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone)
		if err != nil {
			log.Errorf("Failed to get %s disk state when an abort signal is received, err: %v", targetDiskVolumeNamesList[i], err)
		}

		if diskState != "attached" {

			//Wait for disk volume detachment
			//We first wait for the volume to get in detached state then we are attaching it.
			log.Infof("[Abort]: Wait for %s complete disk volume detachment", targetDiskVolumeNamesList[i])

			if err = gcp.WaitForVolumeDetachment(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, instanceNamesList[i], zone, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				log.Errorf("Unable to detach %s disk volume, err: %v", targetDiskVolumeNamesList[i], err)
			}

			//Attaching the disk volume from the instance
			log.Infof("[Chaos]: Attaching %s disk volume to the instance", targetDiskVolumeNamesList[i])

			err = gcp.DiskVolumeAttach(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, zone, experimentsDetails.DeviceNamesList[i], targetDiskVolumeNamesList[i])
			if err != nil {
				log.Errorf("%s disk attachment failed when an abort signal is received, err: %v", targetDiskVolumeNamesList[i], err)
			}
		}

		common.SetTargets(targetDiskVolumeNamesList[i], "reverted", "DiskVolume", chaosDetails)
	}

	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}

// getDeviceNamesAndVMInstanceNames fetches the device name and attached VM instance name for each target disk
func getDeviceNamesAndVMInstanceNames(diskVolumeNamesList []string, computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails) error {

	for i := range diskVolumeNamesList {

		instanceName, err := gcp.GetVolumeAttachmentDetails(computeService, experimentsDetails.GCPProjectID, experimentsDetails.Zones, diskVolumeNamesList[i])
		if err != nil || instanceName == "" {
			return stacktrace.Propagate(err, "failed to get the disk attachment info")
		}

		deviceName, err := gcp.GetDiskDeviceNameForVM(computeService, diskVolumeNamesList[i], experimentsDetails.GCPProjectID, experimentsDetails.Zones, instanceName)
		if err != nil {
			return stacktrace.Propagate(err, "failed to fetch the disk device name")
		}

		experimentsDetails.TargetDiskInstanceNamesList = append(experimentsDetails.TargetDiskInstanceNamesList, instanceName)
		experimentsDetails.DeviceNamesList = append(experimentsDetails.DeviceNamesList, deviceName)
	}

	return nil
}
