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
	AppNS                         string
	AppLabel                      string
	AppKind                       string
	ChaosUID                      clientTypes.UID
	InstanceID                    string
	ChaosNamespace                string
	ChaosPodName                  string
	TargetContainer               string
	FillPercentage                string
	ContainerRuntime              string
	SocketPath                    string
	RunID                         string
	Timeout                       int
	Delay                         int
	LIBImage                      string
	LIBImagePullPolicy            string
	TargetPods                    string
	PodsAffectedPerc              string
	Sequence                      string
	ChaosServiceAccount           string
	EphemeralStorageMebibytes     string
	TerminationGracePeriodSeconds int
	DataBlockSize                 int
	NodeLabel                     string
	IsTargetContainerProvided     bool
	SetHelperData                 string
}
