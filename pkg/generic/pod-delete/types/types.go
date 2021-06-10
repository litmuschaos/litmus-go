package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName      string
	EngineName          string
	ChaosDuration       int
	ChaosInterval       string
	RampTime            int
	Force               bool
	ChaosLib            string
	ChaosServiceAccount string
	AppNS               string
	AppLabel            string
	AppKind             string
	ChaosUID            clientTypes.UID
	InstanceID          string
	ChaosNamespace      string
	ChaosPodName        string
	Timeout             int
	Delay               int
	TargetPods          string
	PodsAffectedPerc    int
	Sequence            string
	LIBImagePullPolicy  string
	TargetContainer     string
}
