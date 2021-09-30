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
	TerminationGracePeriodSeconds int
	InstanceID                    string
	ChaosNamespace                string
	ChaosPodName                  string
	NodeCPUcores                  int
	RunID                         string
	LIBImage                      string
	LIBImagePullPolicy            string
	AuxiliaryAppInfo              string
	Timeout                       int
	Delay                         int
	TargetNodes                   string
	NodesAffectedPerc             int
	Sequence                      string
	TargetContainer               string
	NodeLabel                     string
}
