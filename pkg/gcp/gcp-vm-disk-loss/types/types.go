package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName           string
	EngineName               string
	ChaosDuration            int
	ChaosInterval            int
	RampTime                 int
	ChaosLib                 string
	AppNS                    string
	AppLabel                 string
	AppKind                  string
	ChaosUID                 clientTypes.UID
	InstanceID               string
	ChaosNamespace           string
	ChaosPodName             string
	AuxiliaryAppInfo         string
	Timeout                  int
	Delay                    int
	Sequence                 string
	TargetContainer          string
	LIBImagePullPolicy       string
	GCPProjectID             string
	DiskVolumeNames          string
	DiskZones                string
	DeviceNames              string
	TargetDiskVolumeNameList []string
}
