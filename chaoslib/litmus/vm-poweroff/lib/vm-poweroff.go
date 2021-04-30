package lib

import (
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/types"
	vmwarelib "github.com/litmuschaos/litmus-go/pkg/cloud/vmware"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/litmuschaos/litmus-go/pkg/types"
        "github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/result"
)

func InjectVMPowerOffChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, cookie string) error {

	var err error
	
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
	
	if experimentsDetails.AppVmMoid == "" {
		return errors.Errorf("VM Moid not found to terminate vm instance")
	}

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go AbortWatcher(experimentsDetails, clients, resultDetails, chaosDetails, eventsDetails, cookie)

	//Stoping the vmware VM
	log.Info("[Chaos]: Stoping the desired vm")
	err = vmwarelib.StopVM(experimentsDetails, cookie)
	if err != nil {
		return errors.Errorf("Unable to stop the vm, err: %v", err)
	}

        // run the probes during chaos
        if len(resultDetails.ProbeDetails) != 0 {
                if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
                        return err
                }
        }

	//Wait for chaos duration
	log.Infof("[Wait]: Waiting for chaos duration of %vs before starting the instance", experimentsDetails.ChaosDuration)
	time.Sleep(time.Duration(experimentsDetails.ChaosDuration) * time.Second)	
	

	//Starting the vmware VM
	log.Info("[Chaos]: Starting the desired vm")
	err = vmwarelib.StartVM(experimentsDetails, cookie)
	if err != nil {
		return errors.Errorf("Unable to start the vm, err: %v", err)
	}
	
	
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}	
	
	return nil
}
// AbortWatcher continuosly watch for the abort signals
// it will update chaosresult and restart the vm w/ failed step and create an abort event, if it recieved abort signal during chaos
func AbortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails, cookie string) {
	AbortWatcherWithoutExit(experimentsDetails, clients, resultDetails, chaosDetails, eventsDetails, cookie)
	os.Exit(1)
}

// AbortWatcherWithoutExit continuosly watch for the abort signals
func AbortWatcherWithoutExit(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails, cookie string) {

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM)

loop:
	for {
		select {
		case <-signChan:

		        //Starting the vmware VM
		        log.Info("[Abort]: Abort signal received starting the desired vm")
			err := vmwarelib.StartVM(experimentsDetails, cookie)
		        if err != nil {
		                log.Errorf("Unable to start the vm after abort signal received , err: %v", err)
		        }

			log.Info("[Chaos]: Chaos Experiment Abortion started because of terminated signal received")
			// updating the chaosresult after stopped
			failStep := "Chaos injection stopped!"
			types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
			result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

			// generating summary event in chaosengine
			msg := experimentsDetails.ExperimentName + " experiment has been aborted"
			types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

			// generating summary event in chaosresult
			types.SetResultEventAttributes(eventsDetails, types.Summary, msg, "Warning", resultDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")
			break loop
		}
	}
}
