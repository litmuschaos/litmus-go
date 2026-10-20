package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName   string
	EngineName       string
	ChaosDuration    int
	RampTime         int
	TargetContainer  string
	ChaosUID         clientTypes.UID
	InstanceID       string
	ChaosNamespace   string
	ChaosPodName     string
	AuxiliaryAppInfo string
	Timeout          int
	Delay            int
	IPMIIP           string
	User             string
	Password         string
}
