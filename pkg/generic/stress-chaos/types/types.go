package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName                  string
	EngineName                      string
	ChaosDuration                   int
	LIBImage                        string
	LIBImagePullPolicy              string
	RampTime                        int
	AppNS                           string
	AppLabel                        string
	AppKind                         string
	ChaosUID                        clientTypes.UID
	InstanceID                      string
	ChaosNamespace                  string
	ChaosPodName                    string
	RunID                           string
	TargetContainer                 string
	Timeout                         int
	Delay                           int
	TargetPods                      string
	PodsAffectedPerc                string
	ContainerRuntime                string
	ChaosServiceAccount             string
	SocketPath                      string
	Sequence                        string
	TerminationGracePeriodSeconds   int
	CPUcores                        string
	CPULoad                         string
	FilesystemUtilizationPercentage string
	FilesystemUtilizationBytes      string
	NumberOfWorkers                 string
	MemoryConsumption               string
	VolumeMountPath                 string
	StressType                      string
	IsTargetContainerProvided       bool
	NodeLabel                       string
	SetHelperData                   string
}
