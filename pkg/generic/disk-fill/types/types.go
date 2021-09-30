package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName                string
	EngineName                    string
	ChaosDuration                 int
	RampTime                      int
	ChaosLib                      string
	AppNS                         string
	AppLabel                      string
	AppKind                       string
	ChaosUID                      clientTypes.UID
	InstanceID                    string
	ChaosNamespace                string
	ChaosPodName                  string
	TargetContainer               string
	AuxiliaryAppInfo              string
	FillPercentage                int
	ContainerPath                 string
	RunID                         string
	Timeout                       int
	Delay                         int
	LIBImage                      string
	LIBImagePullPolicy            string
	TargetPods                    string
	PodsAffectedPerc              int
	Sequence                      string
	ChaosServiceAccount           string
	EphemeralStorageMebibytes     int
	TerminationGracePeriodSeconds int
	DataBlockSize                 int
}
