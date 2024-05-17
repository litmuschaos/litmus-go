package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName                  string
	EngineName                      string
	ChaosDuration                   int
	RampTime                        int
	ChaosUID                        clientTypes.UID
	InstanceID                      string
	TerminationGracePeriodSeconds   int
	ChaosNamespace                  string
	ChaosPodName                    string
	RunID                           string
	LIBImage                        string
	LIBImagePullPolicy              string
	AuxiliaryAppInfo                string
	Timeout                         int
	Delay                           int
	TargetNodes                     string
	FilesystemUtilizationPercentage string
	FilesystemUtilizationBytes      string
	CPU                             string
	NumberOfWorkers                 string
	VMWorkers                       string
	NodesAffectedPerc               string
	Sequence                        string
	TargetContainer                 string
	NodeLabel                       string
	SetHelperData                   string
}
