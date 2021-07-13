package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName     string
	EngineName         string
	ChaosDuration      int
	ChaosInterval      int
	RampTime           int
	ChaosLib           string
	AppNS              string
	AuxiliaryAppInfo   string
	AppLabel           string
	AppKind            string
	ChaosUID           clientTypes.UID
	InstanceID         string
	ChaosNamespace     string
	ChaosPodName       string
	Timeout            int
	Delay              int
	VMInstanceName     string
	GCPProjectID       string
	InstanceZone       string
	AutoScalingGroup   string
	Sequence           string
	TargetContainer    string
	LIBImagePullPolicy string
}
