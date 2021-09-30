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
	ChaosLib                           string
	AppNS                              string
	AppLabel                           string
	AppKind                            string
	ChaosUID                           clientTypes.UID
	InstanceID                         string
	ChaosNamespace                     string
	ChaosPodName                       string
	RunID                              string
	NetworkPacketDuplicationPercentage int
	NetworkInterface                   string
	TargetContainer                    string
	NetworkLatency                     int
	NetworkPacketLossPercentage        int
	NetworkPacketCorruptionPercentage  int
	TCImage                            string
	Timeout                            int
	Delay                              int
	TargetPods                         string
	PodsAffectedPerc                   int
	DestinationIPs                     string
	DestinationHosts                   string
	ContainerRuntime                   string
	ChaosServiceAccount                string
	SocketPath                         string
	Sequence                           string
	TerminationGracePeriodSeconds      int
}
