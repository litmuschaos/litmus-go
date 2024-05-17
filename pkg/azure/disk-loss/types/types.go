package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName   string
	EngineName       string
	ChaosDuration    int
	ChaosInterval    int
	RampTime         int
	ChaosUID         clientTypes.UID
	InstanceID       string
	ChaosNamespace   string
	ChaosPodName     string
	Timeout          int
	Delay            int
	ScaleSet         string
	ResourceGroup    string
	SubscriptionID   string
	VirtualDiskNames string
	Sequence         string
}
