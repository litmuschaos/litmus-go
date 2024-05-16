package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ADD THE ATTRIBUTES OF YOUR CHOICE HERE
// FEW MANDATORY ATTRIBUTES ARE ADDED BY DEFAULT

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName string
	EngineName     string
	ChaosDuration  int
	ChaosInterval  int
	RampTime       int
	ChaosUID       clientTypes.UID
	InstanceID     string
	ChaosNamespace string
	ChaosPodName   string
	Timeout        int
	Delay          int
	Sequence       string
	VMIds          string
	VMTag          string
	VcenterServer  string
	VcenterUser    string
	VcenterPass    string
}
