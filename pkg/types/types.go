package types

import clientTypes "k8s.io/apimachinery/pkg/types"

const (
	// PreChaosCheck initial stage of experiment check for health before chaos injection
	PreChaosCheck string = "PreChaosCheck"
	// PostChaosCheck  pre-final stage of experiment check for health after chaos injection
	PostChaosCheck string = "PostChaosCheck"
	// Summary final stage of experiment update the verdict
	Summary string = "Summary"
	// ChaosInject this stage refer to the main chaos injection
	ChaosInject string = "ChaosInject"
)

// ResultDetails is for collecting all the chaos-result-related details
type ResultDetails struct {
	Name      string
	Verdict   string
	FailStep  string
	Phase     string
	ResultUID clientTypes.UID
}

// EventDetails is for collecting all the events-related details
type EventDetails struct {
	Message      string
	Reason       string
	ResourceName string
	ResourceUID  clientTypes.UID
	Type         string
}

// ChaosDetails is for collecting all the global variables
type ChaosDetails struct {
	ChaosUID       clientTypes.UID
	ChaosNamespace string
	ChaosPodName   string
	EngineName     string
	InstanceID     string
	ExperimentName string
}

//SetResultAttributes initialise all the chaos result ENV
func SetResultAttributes(resultDetails *ResultDetails, EngineName string, ExperimentName string) {
	resultDetails.Verdict = "Awaited"
	resultDetails.Phase = "Running"
	resultDetails.FailStep = "N/A"
	if EngineName != "" {
		resultDetails.Name = EngineName + "-" + ExperimentName
	} else {
		resultDetails.Name = ExperimentName
	}
}

//SetResultAfterCompletion set all the chaos result ENV in the EOT
func SetResultAfterCompletion(resultDetails *ResultDetails, verdict, phase, failStep string) {
	resultDetails.Verdict = verdict
	resultDetails.Phase = phase
	resultDetails.FailStep = failStep
}

//SetEngineEventAttributes initialise attributes for event generation in chaos engine
func SetEngineEventAttributes(eventsDetails *EventDetails, Reason, Message, Type string, chaosDetails *ChaosDetails) {

	eventsDetails.Reason = Reason
	eventsDetails.Message = Message
	eventsDetails.ResourceName = chaosDetails.EngineName
	eventsDetails.ResourceUID = chaosDetails.ChaosUID
	eventsDetails.Type = Type

}

//SetResultEventAttributes initialise attributes for event generation in chaos result
func SetResultEventAttributes(eventsDetails *EventDetails, Reason string, Message string, resultDetails *ResultDetails) {

	eventsDetails.Reason = Reason
	eventsDetails.Message = Message
	eventsDetails.ResourceName = resultDetails.Name
	eventsDetails.ResourceUID = resultDetails.ResultUID

}
