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

//SetEventAttributes initialise all the chaos result ENV
func SetEventAttributes(eventsDetails *types.EventDetails, Reason string, Message string) {

	eventsDetails.Reason = Reason
	eventsDetails.Message = Message
}
