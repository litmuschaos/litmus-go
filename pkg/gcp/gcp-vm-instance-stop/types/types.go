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
	ChaosUID                 clientTypes.UID
	InstanceID               string
	ChaosNamespace           string
	ChaosPodName             string
	Timeout                  int
	Delay                    int
	VMInstanceName           string
	GCPProjectID             string
	InstanceZone             string
	ManagedInstanceGroup     string
	Sequence                 string
	TargetContainer          string
	InstanceLabel            string
	InstanceAffectedPerc     int
	TargetVMInstanceNameList []string
}
