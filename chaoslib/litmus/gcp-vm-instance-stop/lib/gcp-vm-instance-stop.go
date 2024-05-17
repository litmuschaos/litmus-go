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
	gcplib "github.com/litmuschaos/litmus-go/pkg/cloud/gcp"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-instance-stop/types"
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

// PrepareVMStop contains the prepration and injection steps for the experiment
func PrepareVMStop(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	// waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// get the instance name or list of instance names
	instanceNamesList := strings.Split(experimentsDetails.VMInstanceName, ",")

	// get the zone name or list of corresponding zones for the instances
	instanceZonesList := strings.Split(experimentsDetails.Zones, ",")

	go abortWatcher(computeService, experimentsDetails, instanceNamesList, instanceZonesList, chaosDetails)

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(computeService, experimentsDetails, instanceNamesList, instanceZonesList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err = injectChaosInParallelMode(computeService, experimentsDetails, instanceNamesList, instanceZonesList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
	}

	// wait for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	return nil
}

// injectChaosInSerialMode stops VM instances in serial mode i.e. one after the other
func injectChaosInSerialMode(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, instanceNamesList []string, instanceZonesList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

			log.Infof("[Info]: Target instance list, %v", instanceNamesList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos in VM instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			//Stop the instance
			for i := range instanceNamesList {

				//Stopping the VM instance
				log.Infof("[Chaos]: Stopping %s VM instance", instanceNamesList[i])
				if err := gcplib.VMInstanceStop(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
					return stacktrace.Propagate(err, "vm instance failed to stop")
				}

				common.SetTargets(instanceNamesList[i], "injected", "VM", chaosDetails)

				//Wait for VM instance to completely stop
				log.Infof("[Wait]: Wait for VM instance %s to get in stopped state", instanceNamesList[i])
				if err := gcplib.WaitForVMInstanceDown(computeService, experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
					return stacktrace.Propagate(err, "vm instance failed to fully shutdown")
				}

				// run the probes during chaos
				// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
				if len(resultDetails.ProbeDetails) != 0 && i == 0 {
					if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
						return err
					}
				}

				// wait for the chaos interval
				log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
				common.WaitForDuration(experimentsDetails.ChaosInterval)

				switch experimentsDetails.ManagedInstanceGroup {
				case "disable":

					// starting the VM instance
					log.Infof("[Chaos]: Starting back %s VM instance", instanceNamesList[i])
					if err := gcplib.VMInstanceStart(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
						return stacktrace.Propagate(err, "vm instance failed to start")
					}

					// wait for VM instance to get in running state
					log.Infof("[Wait]: Wait for VM instance %s to get in running state", instanceNamesList[i])
					if err := gcplib.WaitForVMInstanceUp(computeService, experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
						return stacktrace.Propagate(err, "unable to start vm instance")
					}

				default:

					// wait for VM instance to get in running state
					log.Infof("[Wait]: Wait for VM instance %s to get in running state", instanceNamesList[i])
					if err := gcplib.WaitForVMInstanceUp(computeService, experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
						return stacktrace.Propagate(err, "unable to start vm instance")
					}
				}

				common.SetTargets(instanceNamesList[i], "reverted", "VM", chaosDetails)
			}

			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}

	return nil
}

// injectChaosInParallelMode stops VM instances in parallel mode i.e. all at once
func injectChaosInParallelMode(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, instanceNamesList []string, instanceZonesList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

			log.Infof("[Info]: Target VM instance list, %v", instanceNamesList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos in VM instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			// power-off the instance
			for i := range instanceNamesList {

				// stopping the VM instance
				log.Infof("[Chaos]: Stopping %s VM instance", instanceNamesList[i])
				if err := gcplib.VMInstanceStop(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
					return stacktrace.Propagate(err, "vm instance failed to stop")
				}

				common.SetTargets(instanceNamesList[i], "injected", "VM", chaosDetails)
			}

			for i := range instanceNamesList {

				// wait for VM instance to completely stop
				log.Infof("[Wait]: Wait for VM instance %s to get in stopped state", instanceNamesList[i])
				if err := gcplib.WaitForVMInstanceDown(computeService, experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
					return stacktrace.Propagate(err, "vm instance failed to fully shutdown")
				}
			}

			// run the probes during chaos
			if len(resultDetails.ProbeDetails) != 0 {
				if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return err
				}
			}

			// wait for chaos interval
			log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)

			switch experimentsDetails.ManagedInstanceGroup {
			case "disable":

				// starting the VM instance
				for i := range instanceNamesList {
					log.Infof("[Chaos]: Starting back %s VM instance", instanceNamesList[i])
					if err := gcplib.VMInstanceStart(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
						return stacktrace.Propagate(err, "vm instance failed to start")
					}
				}

				// wait for VM instance to get in running state
				for i := range instanceNamesList {

					log.Infof("[Wait]: Wait for VM instance %s to get in running state", instanceNamesList[i])
					if err := gcplib.WaitForVMInstanceUp(computeService, experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
						return stacktrace.Propagate(err, "unable to start vm instance")
					}

					common.SetTargets(instanceNamesList[i], "reverted", "VM", chaosDetails)
				}

			default:

				// wait for VM instance to get in running state
				for i := range instanceNamesList {

					log.Infof("[Wait]: Wait for VM instance %s to get in running state", instanceNamesList[i])
					if err := gcplib.WaitForVMInstanceUp(computeService, experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
						return stacktrace.Propagate(err, "unable to start vm instance")
					}

					common.SetTargets(instanceNamesList[i], "reverted", "VM", chaosDetails)
				}
			}

			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}

	return nil
}

// abortWatcher watches for the abort signal and reverts the chaos
func abortWatcher(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails, instanceNamesList []string, zonesList []string, chaosDetails *types.ChaosDetails) {
	<-abort

	log.Info("[Abort]: Chaos Revert Started")

	if experimentsDetails.ManagedInstanceGroup != "enable" {

		for i := range instanceNamesList {

			instanceState, err := gcplib.GetVMInstanceStatus(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, zonesList[i])
			if err != nil {
				log.Errorf("Failed to get %s vm instance status when an abort signal is received, err: %v", instanceNamesList[i], err)
			}

			if instanceState != "RUNNING" {

				log.Infof("[Abort]: Waiting for %s VM instance to shut down", instanceNamesList[i])
				if err := gcplib.WaitForVMInstanceDown(computeService, experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, zonesList[i]); err != nil {
					log.Errorf("Unable to wait till stop of %s instance, err: %v", instanceNamesList[i], err)
				}

				log.Infof("[Abort]: Starting %s VM instance as abort signal is received", instanceNamesList[i])
				err := gcplib.VMInstanceStart(computeService, instanceNamesList[i], experimentsDetails.GCPProjectID, zonesList[i])
				if err != nil {
					log.Errorf("%s VM instance failed to start when an abort signal is received, err: %v", instanceNamesList[i], err)
				}
			}

			common.SetTargets(instanceNamesList[i], "reverted", "VM", chaosDetails)
		}
	}

	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
