package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName                string
	EngineName                    string
	ChaosDuration                 int
	LIBImage                      string
	LIBImagePullPolicy            string
	RampTime                      int
	ChaosLib                      string
	AppNS                         string
	AppLabel                      string
	AppKind                       string
	ChaosUID                      clientTypes.UID
	InstanceID                    string
	ChaosNamespace                string
	ChaosPodName                  string
	RunID                         string
	Timeout                       int
	Delay                         int
	TargetContainer               string
	TargetPods                    string
	PodsAffectedPerc              int
	TargetHostNames               string
	SpoofMap                      string
	MatchScheme                   string
	ChaosType                     string
	ContainerRuntime              string
	ChaosServiceAccount           string
	Sequence                      string
	SocketPath                    string
	TerminationGracePeriodSeconds int
}
