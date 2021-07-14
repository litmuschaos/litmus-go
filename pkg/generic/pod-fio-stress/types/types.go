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
	AppLabel           string
	AppKind            string
	ChaosUID           clientTypes.UID
	InstanceID         string
	ChaosNamespace     string
	ChaosPodName       string
	Timeout            int
	Delay              int
	TargetContainer    string
	ChaosInjectCmd     string
	ChaosKillCmd       string
	PodsAffectedPerc   int
	TargetPods         string
	LIBImagePullPolicy string
	Sequence           string
	IOEngine           string
	IODepth            int
	ReadWrite          string
	BlockSize          string
	Size               string
	NumJobs            int
	GroupReporting     bool
}
