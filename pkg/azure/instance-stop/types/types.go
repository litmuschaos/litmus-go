package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName     string
	EngineName         string
	RampTime           int
	ChaosDuration      int
	ChaosInterval      int
	ChaosUID           clientTypes.UID
	InstanceID         string
	ChaosNamespace     string
	ChaosPodName       string
	Timeout            int
	Delay              int
	AzureInstanceNames string
	ResourceGroup      string
	SubscriptionID     string
	ScaleSet           string
	Sequence           string
}
