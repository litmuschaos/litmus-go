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
	IsTargetContainerProvided     bool
	Timeout                       int
	Delay                         int
	TerminationGracePeriodSeconds int
	TargetPods                    string
	PodsAffectedPerc              string
	ContainerRuntime              string
	ChaosServiceAccount           string
	SocketPath                    string
	SetHelperData                 string
	Sequence                      string
	NodeLabel                     string
	NetworkInterface              string
	TargetServicePort             int
	ProxyPort                     int
	Latency                       int
	StatusCode                    int
	ModifyResponseBody            string
	HeadersMap                    string
	HeaderMode                    string
	ResponseBody                  string
	ResetTimeout                  int
}
