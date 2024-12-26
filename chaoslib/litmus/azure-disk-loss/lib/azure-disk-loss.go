package lib

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/disk-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	diskStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure/disk"
	instanceStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure/instance"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/palantir/stacktrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var (
	err           error
	inject, abort chan os.Signal
)

// PrepareChaos contains the prepration and injection steps for the experiment
func PrepareChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PrepareAzureDiskLossFault")
	defer span.End()

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
	if experimentsDetails.VirtualDiskNames == "" || len(diskNameList) == 0 {
		span.SetStatus(codes.Error, "no volume names found to detach")
		err := cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "no volume names found to detach"}
		span.RecordError(err)
		return err
	}
	instanceNamesWithDiskNames, err := diskStatus.GetInstanceNameForDisks(diskNameList, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup)

	if err != nil {
		span.SetStatus(codes.Error, "failed to get instance names for disks")
		span.RecordError(err)
		return stacktrace.Propagate(err, "error fetching attached instances for disks")
	}

	// Get the instance name with attached disks
	attachedDisksWithInstance := make(map[string]*[]compute.DataDisk)

	for instanceName := range instanceNamesWithDiskNames {
		attachedDisksWithInstance[instanceName], err = diskStatus.GetInstanceDiskList(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.ScaleSet, instanceName)
		if err != nil {
			span.SetStatus(codes.Error, "failed to get attached disks")
			span.RecordError(err)
			return stacktrace.Propagate(err, "error fetching virtual disks")
		}
	}

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:

		// watching for the abort signal and revert the chaos
		go abortWatcher(experimentsDetails, attachedDisksWithInstance, instanceNamesWithDiskNames, chaosDetails)

		switch strings.ToLower(experimentsDetails.Sequence) {
		case "serial":
			if err = injectChaosInSerialMode(ctx, experimentsDetails, instanceNamesWithDiskNames, attachedDisksWithInstance, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				span.SetStatus(codes.Error, "failed to run chaos in serial mode")
				span.RecordError(err)
				return stacktrace.Propagate(err, "could not run chaos in serial mode")
			}
		case "parallel":
			if err = injectChaosInParallelMode(ctx, experimentsDetails, instanceNamesWithDiskNames, attachedDisksWithInstance, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				span.SetStatus(codes.Error, "failed to run chaos in parallel mode")
				span.RecordError(err)
				return stacktrace.Propagate(err, "could not run chaos in parallel mode")
			}
		default:
			span.SetStatus(codes.Error, "sequence is not supported")
			err := cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
			span.RecordError(err)
			return err
		}

		//Waiting for the ramp time after chaos injection
		if experimentsDetails.RampTime != 0 {
			log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
			common.WaitForDuration(experimentsDetails.RampTime)
		}
	}
	return nil
}

// injectChaosInParallelMode will inject the Azure disk loss chaos in parallel mode that is all at once
func injectChaosInParallelMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, instanceNamesWithDiskNames map[string][]string, attachedDisksWithInstance map[string]*[]compute.DataDisk, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectAzureDiskLossFaultInParallelMode")
	defer span.End()

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on Azure virtual disk"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		// Detaching the virtual disks
		log.Info("[Chaos]: Detaching the virtual disks from the instances")
		for instanceName, diskNameList := range instanceNamesWithDiskNames {
			if err = diskStatus.DetachDisks(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet, diskNameList); err != nil {
				span.SetStatus(codes.Error, "failed to detach disks")
				span.RecordError(err)
				return stacktrace.Propagate(err, "failed to detach disks")
			}
		}
		// Waiting for disk to be detached
		for _, diskNameList := range instanceNamesWithDiskNames {
			for _, diskName := range diskNameList {
				log.Infof("[Wait]: Waiting for Disk '%v' to detach", diskName)
				if err := diskStatus.WaitForDiskToDetach(experimentsDetails, diskName); err != nil {
					span.SetStatus(codes.Error, "failed to detach disks")
					span.RecordError(err)
					return stacktrace.Propagate(err, "disk detachment check failed")
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
			if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
				span.SetStatus(codes.Error, "failed to run probes")
				span.RecordError(err)
				return stacktrace.Propagate(err, "failed to run probes")
			}
		}

		//Wait for chaos duration
		log.Infof("[Wait]: Waiting for the chaos interval of %vs", experimentsDetails.ChaosInterval)
		common.WaitForDuration(experimentsDetails.ChaosInterval)

		//Attaching the virtual disks to the instance
		log.Info("[Chaos]: Attaching the Virtual disks back to the instances")
		for instanceName, diskNameList := range attachedDisksWithInstance {
			if err = diskStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet, diskNameList); err != nil {
				span.SetStatus(codes.Error, "virtual disk attachment failed")
				span.RecordError(err)
				return stacktrace.Propagate(err, "virtual disk attachment failed")
			}

			// Wait for disk to be attached
			for _, diskNameList := range instanceNamesWithDiskNames {
				for _, diskName := range diskNameList {
					log.Infof("[Wait]: Waiting for Disk '%v' to attach", diskName)
					if err := diskStatus.WaitForDiskToAttach(experimentsDetails, diskName); err != nil {
						span.SetStatus(codes.Error, "failed to attach disks")
						span.RecordError(err)
						return stacktrace.Propagate(err, "disk attachment check failed")
					}
				}
			}

			// Updating the result details
			for _, diskNameList := range instanceNamesWithDiskNames {
				for _, diskName := range diskNameList {
					common.SetTargets(diskName, "re-attached", "VirtualDisk", chaosDetails)
				}
			}
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

// injectChaosInSerialMode will inject the Azure disk loss chaos in serial mode that is one after other
func injectChaosInSerialMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, instanceNamesWithDiskNames map[string][]string, attachedDisksWithInstance map[string]*[]compute.DataDisk, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectAzureDiskLossFaultInSerialMode")
	defer span.End()

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on Azure virtual disks"
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
					span.SetStatus(codes.Error, "failed to detach disks")
					span.RecordError(err)
					return stacktrace.Propagate(err, "failed to detach disks")
				}

				// Waiting for disk to be detached
				log.Infof("[Wait]: Waiting for Disk '%v' to detach", diskName)
				if err := diskStatus.WaitForDiskToDetach(experimentsDetails, diskName); err != nil {
					span.SetStatus(codes.Error, "failed to detach disks")
					span.RecordError(err)
					return stacktrace.Propagate(err, "disk detachment check failed")
				}

				common.SetTargets(diskName, "detached", "VirtualDisk", chaosDetails)

				// run the probes during chaos
				// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
				if len(resultDetails.ProbeDetails) != 0 && i == 0 {
					if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
						return stacktrace.Propagate(err, "failed to run probes")
					}
				}

				//Wait for chaos duration
				log.Infof("[Wait]: Waiting for the chaos interval of %vs", experimentsDetails.ChaosInterval)
				common.WaitForDuration(experimentsDetails.ChaosInterval)

				//Attaching the virtual disks to the instance
				log.Infof("[Chaos]: Attaching %v back to the instance", diskName)
				if err = diskStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet, attachedDisksWithInstance[instanceName]); err != nil {
					span.SetStatus(codes.Error, "disk attachment failed")
					span.RecordError(err)
					return stacktrace.Propagate(err, "disk attachment failed")
				}

				// Waiting for disk to be attached
				log.Infof("[Wait]: Waiting for Disk '%v' to attach", diskName)
				if err := diskStatus.WaitForDiskToAttach(experimentsDetails, diskName); err != nil {
					span.SetStatus(codes.Error, "failed to attach disks")
					span.RecordError(err)
					return stacktrace.Propagate(err, "disk attachment check failed")
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

	log.Info("[Abort]: Attaching disk(s) as abort signal received")

	for instanceName, diskList := range attachedDisksWithInstance {
		// Checking for provisioning state of the vm instances
		err = retry.
			Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
			Wait(time.Duration(experimentsDetails.Delay) * time.Second).
			Try(func(attempt uint) error {
				status, err := instanceStatus.GetAzureInstanceProvisionStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet)
				if err != nil {
					return stacktrace.Propagate(err, "failed to get instance")
				}
				if status != "Provisioning succeeded" {
					return stacktrace.Propagate(err, "instance is updating, waiting for instance to finish update")
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
				log.Errorf("Failed to get disk status: %v", err)
			}
			if diskStatusString != "Attached" {
				if err := diskStatus.AttachDisk(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, instanceName, experimentsDetails.ScaleSet, diskList); err != nil {
					log.Errorf("Failed to attach disk, manual revert required: %v", err)
				} else {
					common.SetTargets(*disk.Name, "re-attached", "VirtualDisk", chaosDetails)
				}
			}
		}
	}

	log.Infof("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
