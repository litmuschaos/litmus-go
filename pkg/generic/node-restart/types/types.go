package types

import (
	corev1 "k8s.io/api/core/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName     string
	EngineName         string
	ChaosDuration      int
	Annotations        map[string]string
	RampTime           int
	ChaosLib           string
	AppNS              string
	AppLabel           string
	AppKind            string
	ChaosUID           clientTypes.UID
	InstanceID         string
	ChaosNamespace     string
	ChaosPodName       string
	RunID              string
	LIBImage           string
	LIBImagePullPolicy string
	AuxiliaryAppInfo   string
	Timeout            int
	Delay              int
	SSHUser            string
	RebootCommand      string
	TargetNode         string
	TargetNodeIP       string
	Resources          corev1.ResourceRequirements
}
