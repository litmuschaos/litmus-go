package lib

import (
	"os"
	"os/signal"
	"syscall"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	vmwarelib "github.com/litmuschaos/litmus-go/pkg/cloud/vmware"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/types"
	"github.com/pkg/errors"
)

var inject, abort chan os.Signal

// InjectVMPowerOffChaos stop the vm, wait for chaos duration and start the vm
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

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on vm poweroff"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	if experimentsDetails.VMId == "" {
		return errors.Errorf("VM Moid not found to terminate vm instance")
	}

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go abortWatcher(experimentsDetails, clients, resultDetails, chaosDetails, eventsDetails, cookie)

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		//Stoping the vmware VM
		log.Info("[Chaos]: Stoping the desired vm")
		if err := vmwarelib.StopVM(experimentsDetails.VcenterServer, experimentsDetails.VMId, cookie); err != nil {
			return errors.Errorf("unable to stop the vm, err: %v", err)
		}
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	//Wait for chaos duration
	log.Infof("[Wait]: Waiting for chaos duration of %vs before starting the instance", experimentsDetails.ChaosDuration)
	common.WaitForDuration(experimentsDetails.ChaosDuration)

	//Starting the vmware VM
	log.Info("[Chaos]: Starting the desired vm")
	if err := vmwarelib.StartVM(experimentsDetails.VcenterServer, experimentsDetails.VMId, cookie); err != nil {
		return errors.Errorf("unable to start the vm, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	return nil
}

// abortWatcher continuosly watch for the abort signals
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails, cookie string) {
	// waiting till the abort signal recieved
	<-abort

	log.Info("Chaos Revert Started")

	//Starting the vmware VM
	log.Info("[Abort]: Abort signal received starting the desired vm")
	if err := vmwarelib.StartVM(experimentsDetails.VcenterServer, experimentsDetails.VMId, cookie); err != nil {
		log.Errorf("Unable to start the vm after abort signal received , err: %v", err)
	}

	log.Info("Chaos Revert Completed")
	os.Exit(0)
}
