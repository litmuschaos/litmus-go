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
	ChaosInterval      int
	RampTime           int
	ChaosLib           string
	AppNS              string
	AppLabel           string
	AppKind            string
	ChaosUID           clientTypes.UID
	InstanceID         string
	ChaosNamespace     string
	ChaosPodName       string
	PodsAffectedPerc   int
	MemoryConsumption  int
	Timeout            int
	Delay              int
	TargetPods         string
	ChaosKillCmd       string
	LIBImagePullPolicy string
	Annotations        map[string]string
	TargetContainer    string
	Sequence           string
	Resources          corev1.ResourceRequirements
	ImagePullSecrets   []corev1.LocalObjectReference
}
