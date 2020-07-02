package environment

import (
	types "github.com/litmuschaos/litmus-go/pkg/types"
)

//SetResultAttributes initialise all the chaos result ENV
func SetResultAttributes(resultDetails *types.ResultDetails, EngineName string, ExperimentName string) {
	resultDetails.Verdict = "Awaited"
	resultDetails.Phase = "Running"
	resultDetails.FailStep = "N/A"
	if EngineName != "" {
		resultDetails.Name = EngineName + "-" + ExperimentName
	} else {
		resultDetails.Name = ExperimentName
	}
}

//SetEngineEventAttributes initialise attributes for event generation in chaos engine
func SetEngineEventAttributes(eventsDetails *types.EventDetails, Reason string, Message string, chaosDetails *types.ChaosDetails) {

	eventsDetails.Reason = Reason
	eventsDetails.Message = Message
	eventsDetails.ResourceName = chaosDetails.EngineName
	eventsDetails.ResourceUID = chaosDetails.ChaosUID

}

//SetResultEventAttributes initialise attributes for event generation in chaos result
func SetResultEventAttributes(eventsDetails *types.EventDetails, Reason string, Message string, resultDetails *types.ResultDetails) {

	eventsDetails.Reason = Reason
	eventsDetails.Message = Message
	eventsDetails.ResourceName = resultDetails.Name
	eventsDetails.ResourceUID = resultDetails.ResultUID

}
