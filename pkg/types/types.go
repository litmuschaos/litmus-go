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
