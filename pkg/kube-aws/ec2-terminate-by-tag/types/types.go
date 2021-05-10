package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName       string
	EngineName           string
	RampTime             int
	AppNS                string
	AppLabel             string
	AppKind              string
	AuxiliaryAppInfo     string
	ChaosLib             string
	ChaosDuration        int
	ChaosInterval        int
	ChaosUID             clientTypes.UID
	InstanceID           string
	ChaosNamespace       string
	ChaosPodName         string
	Timeout              int
	Delay                int
	InstanceTag          string
	Region               string
	InstanceAffectedPerc int
	ManagedNodegroup     string
	Sequence             string
	ActiveNodes          int
	LIBImagePullPolicy   string
	TargetContainer      string
	TargetInstanceIDList []string
}
