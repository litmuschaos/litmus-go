package types

import (
	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

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
	ScaleSet                        string
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
	ScriptPath                      string
}

type RunCommandFuture struct {
	VmssFuture compute.VirtualMachineScaleSetVMsRunCommandFuture
	VmFuture   compute.VirtualMachinesRunCommandFuture
}
