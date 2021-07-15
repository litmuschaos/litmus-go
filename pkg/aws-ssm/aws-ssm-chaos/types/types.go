package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName       string
	EngineName           string
	RampTime             int
	AppNS                string
	AppLabel             string
	AppKind              string
	AuxiliaryAppInfo     string
	ChaosLib             string
	ChaosDuration        int
	ChaosInterval        int
	ChaosUID             clientTypes.UID
	InstanceID           string
	ChaosNamespace       string
	ChaosPodName         string
	Timeout              int
	Delay                int
	EC2InstanceID        string
	EC2InstanceTag       string
	Region               string
	InstanceAffectedPerc int
	Sequence             string
	LIBImagePullPolicy   string
	TargetContainer      string
	Cpu                  int
	NumberOfWorkers      int
	MemoryPercentage     int
	InstallDependencies  string
	DocumentName         string
	DocumentType         string
	DocumentFormat       string
	DocumentPath         string
	IsDocsUploaded       bool
	CommandIDs           []string
	TargetInstanceIDList []string
}
