package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ADD THE ATTRIBUTES OF YOUR CHOICE HERE
// FEW MENDATORY ATTRIBUTES ARE ADDED BY DEFAULT

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName                  string
	EngineName                      string
	ChaosDuration                   int
	ChaosInterval                   int
	RampTime                        int
	ChaosLib                        string
	AppNS                           string
	AppLabel                        string
	AppKind                         string
	ChaosUID                        clientTypes.UID
	InstanceID                      string
	TargetContainer                 string
	ChaosNamespace                  string
	ChaosPodName                    string
	Timeout                         int
	Delay                           int
	LIBImagePullPolicy              string
	AzureInstanceNames              string
	ResourceGroup                   string
	SubscriptionID                  string
	IsScaleSet                      string
	Sequence                        string
	ExperimentType                  string
	InstallDependency               string
	OperatingSystem                 string
	CPUcores                        int
	NumberOfWorkers                 int
	MemoryConsumption               int
	FilesystemUtilizationBytes      int
	FilesystemUtilizationPercentage int
	VolumeMountPath                 string
}
