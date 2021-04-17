package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName     string
	EngineName         string
	RampTime           int
	AppNS              string
	AppLabel           string
	AppKind            string
	AuxiliaryAppInfo   string
	ChaosLib           string
	ChaosUID           clientTypes.UID
	InstanceID         string
	ChaosNamespace     string
	ChaosPodName       string
	Timeout            int
	Delay              int
	Region             string
	ActiveNodes        int
	LIBImagePullPolicy string
	TargetContainer    string
	SpotFleetRequestID string
}
