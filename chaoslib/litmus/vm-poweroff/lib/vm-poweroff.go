package lib

import (
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

	//Stoping the vmware VM
	log.Info("[Chaos]: Stoping the desired vm")
	err = vmwarelib.StopVM(experimentsDetails, cookie)
	if err != nil {
		return errors.Errorf("Unable to stop the vm, err: %v", err)
	}
	
	//Wait for chaos duration
	log.Infof("[Wait]: Waiting for chaos duration of %vs before starting the instance", experimentsDetails.ChaosDuration)
	time.Sleep(time.Duration(experimentsDetails.ChaosDuration) * time.Second)	
	
	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}	
	
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
