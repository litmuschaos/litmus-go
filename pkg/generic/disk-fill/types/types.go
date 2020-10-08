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
	ChaosLib         string
	AppNS            string
	AppLabel         string
	AppKind          string
	ChaosUID         clientTypes.UID
	InstanceID       string
	ChaosNamespace   string
	ChaosPodName     string
	TargetContainer  string
	AuxiliaryAppInfo string
	FillPercentage   int
	ContainerPath    string
	RunID            string
	Timeout          int
	Delay            int
	LIBImage         string
	TargetPod        string
	PodsAffectedPerc int
	Annotations      map[string]string
	Sequence         string
}
