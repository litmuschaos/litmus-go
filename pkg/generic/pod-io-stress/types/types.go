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
	ChaosLib                        string
	AppNS                           string
	AppLabel                        string
	AppKind                         string
	ChaosUID                        clientTypes.UID
	InstanceID                      string
	ChaosNamespace                  string
	ChaosPodName                    string
	Timeout                         int
	Delay                           int
	TargetPods                      string
	FilesystemUtilizationPercentage int
	FilesystemUtilizationBytes      int
	NumberOfWorkers                 int
	Annotations                     map[string]string
	LIBImage                        string
	PodsAffectedPerc                int
	Sequence                        string
	VolumeMountPath                 string
}
