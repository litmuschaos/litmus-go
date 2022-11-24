package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName                string
	EngineName                    string
	ChaosDuration                 int
	ChaosInterval                 int
	RampTime                      int
	AppNS                         string
	AppLabel                      string
	AppKind                       string
	ChaosUID                      clientTypes.UID
	TerminationGracePeriodSeconds int
	InstanceID                    string
	ChaosNamespace                string
	ChaosPodName                  string
	LIBImage                      string
	LIBImagePullPolicy            string
	TargetContainer               string
	SocketPath                    string
	ChaosServiceAccount           string
	RunID                         string
	Timeout                       int
	Delay                         int
	TargetPods                    string
	ContainerRuntime              string
	PodsAffectedPerc              string
	Sequence                      string
	Signal                        string
	NodeLabel                     string
	IsTargetContainerProvided     bool
	SetHelperData                 string
}
