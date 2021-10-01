package lib

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	gcplib "github.com/litmuschaos/litmus-go/pkg/cloud/gcp"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-instance-stop/types"
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

//PrepareVMStop contains the prepration and injection steps for the experiment
func PrepareVMStop(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
	if len(instanceNamesList) == 0 {
		return errors.Errorf("no instance name found to stop")
	}

	// get the zone name or list of corresponding zones for the instances
	instanceZonesList := strings.Split(experimentsDetails.InstanceZone, ",")
	if len(instanceZonesList) == 0 {
		return errors.Errorf("no corresponding zones found for the instances")
	}

	if len(instanceNamesList) != len(instanceZonesList) {
		return errors.Errorf("number of instances is not equal to the number of zones")
	}

	go abortWatcher(experimentsDetails, instanceNamesList, instanceZonesList, chaosDetails)

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(experimentsDetails, instanceNamesList, instanceZonesList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, instanceNamesList, instanceZonesList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
	}

	// wait for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//injectChaosInSerialMode stops VM instances in serial mode i.e. one after the other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceNamesList []string, instanceZonesList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

			log.Infof("[Info]: Target instanceNames list, %v", instanceNamesList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos in VM instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			//Stop the instance
			for i := range instanceNamesList {

				//Stopping the VM instance
				log.Info("[Chaos]: Stopping the desired VM instance")
				if err := gcplib.VMInstanceStop(instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
					return errors.Errorf("VM instance failed to stop, err: %v", err)
				}

				common.SetTargets(instanceNamesList[i], "injected", "VM", chaosDetails)

				//Wait for VM instance to completely stop
				log.Infof("[Wait]: Wait for VM instance '%v' to get in stopped state", instanceNamesList[i])
				if err := gcplib.WaitForVMInstanceDown(experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
					return errors.Errorf("vm instance failed to fully shutdown, err: %v", err)
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

				// starting the VM instance
				if experimentsDetails.AutoScalingGroup != "enable" {
					log.Info("[Chaos]: Starting back the VM instance")
					if err := gcplib.VMInstanceStart(instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
						return errors.Errorf("vm instance failed to start, err: %v", err)
					}

					// wait for VM instance to get in running state
					log.Infof("[Wait]: Wait for VM instance '%v' to get in running state", instanceNamesList[i])
					if err := gcplib.WaitForVMInstanceUp(experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
						return errors.Errorf("unable to start the vm instance, err: %v", err)
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
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceNamesList []string, instanceZonesList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

			log.Infof("[Info]: Target instanceID list, %v", instanceNamesList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos in VM instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			// power-off the instance
			for i := range instanceNamesList {

				// stopping the VM instance
				log.Info("[Chaos]: Stopping the desired VM instance")
				if err := gcplib.VMInstanceStop(instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
					return errors.Errorf("vm instance failed to stop, err: %v", err)
				}

				common.SetTargets(instanceNamesList[i], "injected", "VM", chaosDetails)
			}

			for i := range instanceNamesList {

				// wait for VM instance to completely stop
				log.Infof("[Wait]: Wait for VM instance '%v' to get in stopped state", instanceNamesList[i])
				if err := gcplib.WaitForVMInstanceDown(experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
					return errors.Errorf("vm instance failed to fully shutdown, err: %v", err)
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

			// starting the VM instance
			if experimentsDetails.AutoScalingGroup != "enable" {

				for i := range instanceNamesList {
					log.Info("[Chaos]: Starting back the VM instance")
					if err := gcplib.VMInstanceStart(instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
						return errors.Errorf("vm instance failed to start, err: %v", err)
					}
				}

				for i := range instanceNamesList {
					// wait for VM instance to get in running state
					log.Infof("[Wait]: Wait for VM instance '%v' to get in running state", instanceNamesList[i])
					if err := gcplib.WaitForVMInstanceUp(experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, instanceZonesList[i]); err != nil {
						return errors.Errorf("unable to start the vm instance, err: %v", err)
					}
				}
			}

			for i := range instanceNamesList {
				common.SetTargets(instanceNamesList[i], "reverted", "VM", chaosDetails)
			}
			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}
	return nil
}

// abortWatcher watches for the abort signal and reverts the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, instanceNamesList []string, zonesList []string, chaosDetails *types.ChaosDetails) {
	<-abort

	log.Info("[Abort]: Chaos Revert Started")
	for i := range instanceNamesList {
		instanceState, err := gcplib.GetVMInstanceStatus(instanceNamesList[i], experimentsDetails.GCPProjectID, zonesList[i])
		if err != nil {
			log.Errorf("fail to get instance status when an abort signal is received,err :%v", err)
		}
		if instanceState != "RUNNING" && experimentsDetails.AutoScalingGroup != "enable" {

			log.Info("[Abort]: Waiting for the VM instance to shut down")
			if err := gcplib.WaitForVMInstanceDown(experimentsDetails.Timeout, experimentsDetails.Delay, instanceNamesList[i], experimentsDetails.GCPProjectID, zonesList[i]); err != nil {
				log.Errorf("unable to wait till stop of the instance, err: %v", err)
			}

			log.Info("[Abort]: Starting VM instance as abort signal received")
			err := gcplib.VMInstanceStart(instanceNamesList[i], experimentsDetails.GCPProjectID, zonesList[i])
			if err != nil {
				log.Errorf("vm instance failed to start when an abort signal is received, err: %v", err)
			}
		}
		common.SetTargets(instanceNamesList[i], "reverted", "VM", chaosDetails)
	}
	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
