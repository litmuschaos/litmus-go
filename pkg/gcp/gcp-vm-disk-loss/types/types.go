package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName              string
	EngineName                  string
	ChaosDuration               int
	ChaosInterval               int
	RampTime                    int
	ChaosUID                    clientTypes.UID
	InstanceID                  string
	ChaosNamespace              string
	ChaosPodName                string
	Timeout                     int
	Delay                       int
	Sequence                    string
	TargetContainer             string
	GCPProjectID                string
	DiskVolumeNames             string
	Zones                       string
	DiskVolumeLabel             string
	TargetDiskVolumeNamesList   []string
	TargetDiskInstanceNamesList []string
	DiskAffectedPerc            int
	DeviceNamesList             []string
}
