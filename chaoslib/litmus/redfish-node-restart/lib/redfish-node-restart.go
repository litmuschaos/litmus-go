package lib

import (
	"fmt"
	"time"

	redfishLib "github.com/litmuschaos/litmus-go/pkg/baremetal/redfish"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/baremetal/redfish-node-restart/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
)

//injectChaos initiates node restart chaos on the target node
func injectChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	URL := fmt.Sprintf("https://%v/redfish/v1/Systems/System.Embedded.1/Actions/ComputerSystem.Reset", experimentsDetails.IPMIIP)
	return redfishLib.RebootNode(URL, experimentsDetails.User, experimentsDetails.Password)
}

//experimentExecution function orchestrates the experiment by calling the injectChaos function
func experimentExecution(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.IPMIIP + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	if err := injectChaos(experimentsDetails, clients); err != nil {
		return err
	}

	log.Infof("[Chaos]:Waiting for: %vs", experimentsDetails.ChaosDuration)
	time.Sleep(time.Duration(experimentsDetails.ChaosDuration) * time.Second)
	return nil
}

//PrepareChaos contains the chaos prepration and injection steps
func PrepareChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	//Starting the Redfish node restart experiment
	if err := experimentExecution(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
		return err
	}
	common.SetTargets(experimentsDetails.IPMIIP, "targeted", "node", chaosDetails)
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}
