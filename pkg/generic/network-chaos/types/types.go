package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName                     string
	EngineName                         string
	ChaosDuration                      int
	LIBImage                           string
	LIBImagePullPolicy                 string
	RampTime                           int
	AppNS                              string
	AppLabel                           string
	AppKind                            string
	ChaosUID                           clientTypes.UID
	InstanceID                         string
	ChaosNamespace                     string
	ChaosPodName                       string
	RunID                              string
	NetworkPacketDuplicationPercentage string
	NetworkInterface                   string
	TargetContainer                    string
	NetworkLatency                     int
	NetworkPacketLossPercentage        string
	NetworkPacketCorruptionPercentage  string
	Timeout                            int
	Delay                              int
	TargetPods                         string
	PodsAffectedPerc                   string
	DestinationIPs                     string
	DestinationHosts                   string
	ContainerRuntime                   string
	ChaosServiceAccount                string
	SocketPath                         string
	Sequence                           string
	TerminationGracePeriodSeconds      int
	Jitter                             int
	NetworkChaosType                   string
	NodeLabel                          string
	IsTargetContainerProvided          bool
	SetHelperData                      string
	SourcePorts                        string
	DestinationPorts                   string
}
