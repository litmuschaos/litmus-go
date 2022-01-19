package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

type ExperimentDetails struct {
	ExperimentName     string
	EngineName         string
	ChaosDuration      int
	ChaosInterval      int
	RampTime           int
	ChaosLib           string
	AppNS              string
	AppLabel           string
	AppKind            string
	AuxiliaryAppInfo   string
	ChaosUID           clientTypes.UID
	InstanceID         string
	ChaosNamespace     string
	ChaosPodName       string
	Timeout            int
	Delay              int
	TargetContainer    string
	ChaosInjectCmd     string
	ChaosKillCmd       string
	PodsAffectedPerc   int
	TargetPods         string
	TCImage            string
	LIBImagePullPolicy string
	LIBImage           string
	SocketPath         string
	Region             string
	MinNumberOfIps     int
}
