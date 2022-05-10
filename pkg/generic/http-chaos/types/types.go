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
	TargetContainer               string
	NetworkInterface              string
	Timeout                       int
	Delay                         int
	HttpLatency                   int
	TargetPods                    string
	PodsAffectedPerc              string
	ContainerRuntime              string
	ChaosServiceAccount           string
	SocketPath                    string
	Sequence                      string
	TerminationGracePeriodSeconds int
	HttpChaosType                 string
	NodeLabel                     string
	TargetPort                    int
	DestinationPort               int
	IsTargetContainerProvided     bool
}
