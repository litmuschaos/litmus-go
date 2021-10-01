package lib

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/cloud/vmware"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/types"
	"github.com/pkg/errors"
)

var inject, abort chan os.Signal

// InjectVMPowerOffChaos injects the chaos in serial or parallel mode
func InjectVMPowerOffChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, cookie string) error {

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

	//Fetching the target VM Ids
	vmIdList := strings.Split(experimentsDetails.VMIds, ",")

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go abortWatcher(experimentsDetails, vmIdList, clients, resultDetails, chaosDetails, eventsDetails, cookie)

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err := injectChaosInSerialMode(experimentsDetails, vmIdList, cookie, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	case "parallel":
		if err := injectChaosInParallelMode(experimentsDetails, vmIdList, cookie, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
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

	return nil
}

// injectChaosInSerialMode stops VMs in serial mode i.e. one after the other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, vmIdList []string, cookie string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

			log.Infof("[Info]: Target VM Id list, %v", vmIdList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos in VM"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			for i, vmId := range vmIdList {

				//Stopping the VM
				log.Infof("[Chaos]: Stopping %s VM", vmId)
				if err := vmware.StopVM(experimentsDetails.VcenterServer, vmId, cookie); err != nil {
					return errors.Errorf("failed to stop %s vm: %s", vmId, err.Error())
				}

				common.SetTargets(vmId, "injected", "VM", chaosDetails)

				//Wait for the VM to completely stop
				log.Infof("[Wait]: Wait for VM '%s' to get in POWERED_OFF state", vmId)
				if err := vmware.WaitForVMStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.VcenterServer, vmId, cookie); err != nil {
					return errors.Errorf("vm %s failed to successfully shutdown, err: %s", vmId, err.Error())
				}

				//Run the probes during the chaos
				//The OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
				if len(resultDetails.ProbeDetails) != 0 && i == 0 {
					if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
						return err
					}
				}

				//Wait for chaos interval
				log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
				time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)

				//Starting the VM
				log.Infof("[Chaos]: Starting back %s VM", vmId)
				if err := vmware.StartVM(experimentsDetails.VcenterServer, vmId, cookie); err != nil {
					return errors.Errorf("failed to start back %s vm: %s", vmId, err.Error())
				}

				//Wait for the VM to completely start
				log.Infof("[Wait]: Wait for VM '%s' to get in POWERED_ON state", vmId)
				if err := vmware.WaitForVMStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.VcenterServer, vmId, cookie); err != nil {
					return errors.Errorf("vm %s failed to successfully start, err: %s", vmId, err.Error())
				}

				common.SetTargets(vmId, "reverted", "VM", chaosDetails)
			}

			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}

	return nil
}

// injectChaosInParallelMode stops VMs in parallel mode i.e. all at once
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, vmIdList []string, cookie string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

			log.Infof("[Info]: Target VM Id list, %v", vmIdList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos in VM"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			for _, vmId := range vmIdList {

				//Stopping the VM
				log.Infof("[Chaos]: Stopping %s VM", vmId)
				if err := vmware.StopVM(experimentsDetails.VcenterServer, vmId, cookie); err != nil {
					return errors.Errorf("failed to stop %s vm: %s", vmId, err.Error())
				}

				common.SetTargets(vmId, "injected", "VM", chaosDetails)
			}

			for _, vmId := range vmIdList {

				//Wait for the VM to completely stop
				log.Infof("[Wait]: Wait for VM '%s' to get in POWERED_OFF state", vmId)
				if err := vmware.WaitForVMStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.VcenterServer, vmId, cookie); err != nil {
					return errors.Errorf("vm %s failed to successfully shutdown, err: %s", vmId, err.Error())
				}
			}

			//Running the probes during chaos
			if len(resultDetails.ProbeDetails) != 0 {
				if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return err
				}
			}

			//Waiting for chaos interval
			log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)

			for _, vmId := range vmIdList {

				//Starting the VM
				log.Infof("[Chaos]: Starting back %s VM", vmId)
				if err := vmware.StartVM(experimentsDetails.VcenterServer, vmId, cookie); err != nil {
					return errors.Errorf("failed to start back %s vm: %s", vmId, err.Error())
				}
			}

			for _, vmId := range vmIdList {

				//Wait for the VM to completely start
				log.Infof("[Wait]: Wait for VM '%s' to get in POWERED_ON state", vmId)
				if err := vmware.WaitForVMStart(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.VcenterServer, vmId, cookie); err != nil {
					return errors.Errorf("vm %s failed to successfully start, err: %s", vmId, err.Error())
				}
			}

			for _, vmId := range vmIdList {
				common.SetTargets(vmId, "reverted", "VM", chaosDetails)
			}

			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}

	return nil
}

// abortWatcher watches for the abort signal and reverts the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, vmIdList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails, cookie string) {
	<-abort

	log.Info("[Abort]: Chaos Revert Started")
	for _, vmId := range vmIdList {

		vmStatus, err := vmware.GetVMStatus(experimentsDetails.VcenterServer, vmId, cookie)
		if err != nil {
			log.Errorf("failed to get vm status of %s when an abort signal is received: %s", vmId, err.Error())
		}

		if vmStatus != "POWERED_ON" {

			log.Infof("[Abort]: Waiting for the VM %s to shutdown", vmId)
			if err := vmware.WaitForVMStop(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.VcenterServer, vmId, cookie); err != nil {
				log.Errorf("vm %s failed to successfully shutdown when an abort signal was received: %s", vmId, err.Error())
			}

			log.Infof("[Abort]: Starting %s VM as abort signal has been received", vmId)
			if err := vmware.StartVM(experimentsDetails.VcenterServer, vmId, cookie); err != nil {
				log.Errorf("vm %s failed to start when an abort signal was received: %s", vmId, err.Error())
			}
		}

		common.SetTargets(vmId, "reverted", "VM", chaosDetails)
	}

	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
