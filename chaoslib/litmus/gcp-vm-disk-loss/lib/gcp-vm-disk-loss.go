package lib

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	gcp "github.com/litmuschaos/litmus-go/pkg/cloud/gcp"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-disk-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/palantir/stacktrace"
	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
)

var (
	err           error
	inject, abort chan os.Signal
)

// PrepareDiskVolumeLoss contains the prepration and injection steps for the experiment
func PrepareDiskVolumeLoss(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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

	//get the disk volume names list
	diskNamesList := strings.Split(experimentsDetails.DiskVolumeNames, ",")

	//get the disk zones list
	diskZonesList := strings.Split(experimentsDetails.Zones, ",")

	//get the device names for the given disks
	if err := getDeviceNamesList(computeService, experimentsDetails, diskNamesList, diskZonesList); err != nil {
		return stacktrace.Propagate(err, "failed to fetch the disk device names")
	}

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:

		// watching for the abort signal and revert the chaos
		go abortWatcher(computeService, experimentsDetails, diskNamesList, diskZonesList, abort, chaosDetails)

		switch strings.ToLower(experimentsDetails.Sequence) {
		case "serial":
			if err = injectChaosInSerialMode(computeService, experimentsDetails, diskNamesList, diskZonesList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return stacktrace.Propagate(err, "could not run chaos in serial mode")
			}
		case "parallel":
			if err = injectChaosInParallelMode(computeService, experimentsDetails, diskNamesList, diskZonesList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
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
func injectChaosInSerialMode(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, targetDiskVolumeNamesList, diskZonesList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
			log.Infof("[Chaos]: Detaching %s disk volume from the instance", targetDiskVolumeNamesList[i])
			if err = gcp.DiskVolumeDetach(computeService, experimentsDetails.TargetDiskInstanceNamesList[i], experimentsDetails.GCPProjectID, diskZonesList[i], experimentsDetails.DeviceNamesList[i]); err != nil {
				return stacktrace.Propagate(err, "disk detachment failed")
			}

			common.SetTargets(targetDiskVolumeNamesList[i], "injected", "DiskVolume", chaosDetails)

			//Wait for disk volume detachment
			log.Infof("[Wait]: Wait for %s disk volume detachment", targetDiskVolumeNamesList[i])
			if err = gcp.WaitForVolumeDetachment(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, experimentsDetails.TargetDiskInstanceNamesList[i], diskZonesList[i], experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				return stacktrace.Propagate(err, "unable to detach disk volume from the vm instance")
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
			diskState, err := gcp.GetDiskVolumeState(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, experimentsDetails.TargetDiskInstanceNamesList[i], diskZonesList[i])
			if err != nil {
				return stacktrace.Propagate(err, fmt.Sprintf("failed to get %s disk volume status", targetDiskVolumeNamesList[i]))
			}

			switch diskState {
			case "attached":
				log.Infof("[Skip]: %s disk volume is already attached", targetDiskVolumeNamesList[i])
			default:
				//Attaching the disk volume to the instance
				log.Infof("[Chaos]: Attaching %s disk volume back to the instance", targetDiskVolumeNamesList[i])
				if err = gcp.DiskVolumeAttach(computeService, experimentsDetails.TargetDiskInstanceNamesList[i], experimentsDetails.GCPProjectID, diskZonesList[i], experimentsDetails.DeviceNamesList[i], targetDiskVolumeNamesList[i]); err != nil {
					return stacktrace.Propagate(err, "disk attachment failed")
				}

				//Wait for disk volume attachment
				log.Infof("[Wait]: Wait for %s disk volume attachment", targetDiskVolumeNamesList[i])
				if err = gcp.WaitForVolumeAttachment(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, experimentsDetails.TargetDiskInstanceNamesList[i], diskZonesList[i], experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
					return stacktrace.Propagate(err, "unable to attach disk volume to the vm instance")
				}
			}
			common.SetTargets(targetDiskVolumeNamesList[i], "reverted", "DiskVolume", chaosDetails)
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

// injectChaosInParallelMode will inject the disk loss chaos in parallel mode that means all at once
func injectChaosInParallelMode(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, targetDiskVolumeNamesList, diskZonesList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
			log.Infof("[Chaos]: Detaching %s disk volume from the instance", targetDiskVolumeNamesList[i])
			if err = gcp.DiskVolumeDetach(computeService, experimentsDetails.TargetDiskInstanceNamesList[i], experimentsDetails.GCPProjectID, diskZonesList[i], experimentsDetails.DeviceNamesList[i]); err != nil {
				return stacktrace.Propagate(err, "disk detachment failed")
			}

			common.SetTargets(targetDiskVolumeNamesList[i], "injected", "DiskVolume", chaosDetails)
		}

		for i := range targetDiskVolumeNamesList {

			//Wait for disk volume detachment
			log.Infof("[Wait]: Wait for %s disk volume detachment", targetDiskVolumeNamesList[i])
			if err = gcp.WaitForVolumeDetachment(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, experimentsDetails.TargetDiskInstanceNamesList[i], diskZonesList[i], experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				return stacktrace.Propagate(err, "unable to detach disk volume from the vm instance")
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
			diskState, err := gcp.GetDiskVolumeState(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, experimentsDetails.TargetDiskInstanceNamesList[i], diskZonesList[i])
			if err != nil {
				return errors.Errorf("failed to get the disk status, err: %v", err)
			}

			switch diskState {
			case "attached":
				log.Infof("[Skip]: %s disk volume is already attached", targetDiskVolumeNamesList[i])
			default:
				//Attaching the disk volume to the instance
				log.Infof("[Chaos]: Attaching %s disk volume to the instance", targetDiskVolumeNamesList[i])
				if err = gcp.DiskVolumeAttach(computeService, experimentsDetails.TargetDiskInstanceNamesList[i], experimentsDetails.GCPProjectID, diskZonesList[i], experimentsDetails.DeviceNamesList[i], targetDiskVolumeNamesList[i]); err != nil {
					return stacktrace.Propagate(err, "disk attachment failed")
				}

				//Wait for disk volume attachment
				log.Infof("[Wait]: Wait for %s disk volume attachment", targetDiskVolumeNamesList[i])
				if err = gcp.WaitForVolumeAttachment(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, experimentsDetails.TargetDiskInstanceNamesList[i], diskZonesList[i], experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
					return stacktrace.Propagate(err, "unable to attach disk volume to the vm instance")
				}
			}
			common.SetTargets(targetDiskVolumeNamesList[i], "reverted", "DiskVolume", chaosDetails)
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

// AbortWatcher will watching for the abort signal and revert the chaos
func abortWatcher(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, targetDiskVolumeNamesList, diskZonesList []string, abort chan os.Signal, chaosDetails *types.ChaosDetails) {

	<-abort

	log.Info("[Abort]: Chaos Revert Started")

	for i := range targetDiskVolumeNamesList {

		//Getting the disk volume attachment status
		diskState, err := gcp.GetDiskVolumeState(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, experimentsDetails.TargetDiskInstanceNamesList[i], diskZonesList[i])
		if err != nil {
			log.Errorf("Failed to get %s disk state when an abort signal is received, err: %v", targetDiskVolumeNamesList[i], err)
		}

		if diskState != "attached" {

			//Wait for disk volume detachment
			//We first wait for the volume to get in detached state then we are attaching it.
			log.Infof("[Abort]: Wait for complete disk volume detachment for %s", targetDiskVolumeNamesList[i])

			if err = gcp.WaitForVolumeDetachment(computeService, targetDiskVolumeNamesList[i], experimentsDetails.GCPProjectID, experimentsDetails.TargetDiskInstanceNamesList[i], diskZonesList[i], experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				log.Errorf("Unable to detach %s disk volume, err: %v", targetDiskVolumeNamesList[i], err)
			}

			//Attaching the disk volume from the instance
			log.Infof("[Chaos]: Attaching %s disk volume from the instance", targetDiskVolumeNamesList[i])

			err = gcp.DiskVolumeAttach(computeService, experimentsDetails.TargetDiskInstanceNamesList[i], experimentsDetails.GCPProjectID, diskZonesList[i], experimentsDetails.DeviceNamesList[i], targetDiskVolumeNamesList[i])
			if err != nil {
				log.Errorf("%s disk attachment failed when an abort signal is received, err: %v", targetDiskVolumeNamesList[i], err)
			}
		}

		common.SetTargets(targetDiskVolumeNamesList[i], "reverted", "DiskVolume", chaosDetails)
	}

	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}

// getDeviceNamesList fetches the device names for the target disks
func getDeviceNamesList(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, diskNamesList, diskZonesList []string) error {

	for i := range diskNamesList {

		deviceName, err := gcp.GetDiskDeviceNameForVM(computeService, diskNamesList[i], experimentsDetails.GCPProjectID, diskZonesList[i], experimentsDetails.TargetDiskInstanceNamesList[i])
		if err != nil {
			return err
		}

		experimentsDetails.DeviceNamesList = append(experimentsDetails.DeviceNamesList, deviceName)
	}

	return nil
}
