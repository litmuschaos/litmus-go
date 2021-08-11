package types

import (
	corev1 "k8s.io/api/core/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName                  string
	EngineName                      string
	ChaosDuration                   int
	LIBImage                        string
	LIBImagePullPolicy              string
	RampTime                        int
	ChaosLib                        string
	AppNS                           string
	AppLabel                        string
	AppKind                         string
	ChaosUID                        clientTypes.UID
	InstanceID                      string
	ChaosNamespace                  string
	ChaosPodName                    string
	RunID                           string
	TargetContainer                 string
	StressImage                     string
	Timeout                         int
	Delay                           int
	TargetPods                      string
	PodsAffectedPerc                int
	Annotations                     map[string]string
	ContainerRuntime                string
	ChaosServiceAccount             string
	SocketPath                      string
	Sequence                        string
	Resources                       corev1.ResourceRequirements
	ImagePullSecrets                []corev1.LocalObjectReference
	TerminationGracePeriodSeconds   int
	CPUcores                        int
	FilesystemUtilizationPercentage int
	FilesystemUtilizationBytes      int
	NumberOfWorkers                 int
	MemoryConsumption               int
	VolumeMountPath                 string
	HostNetwork                     bool
	VolMount                        string
}
