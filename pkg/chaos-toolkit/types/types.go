package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName                string
	EngineName                    string
	ChaosDuration                 int
	ChaosInterval                 int
	RampTime                      int
	ChaosLib                      string
	ChaosUID                      clientTypes.UID
	TerminationGracePeriodSeconds int
	InstanceID                    string
	ChaosNamespace                string
	ChaosPodName                  string
	LIBImage                      string
	LIBImagePullPolicy            string
	ChaosServiceAccount           string
	RunID                         string
	Timeout                       int
	Delay                         int
	Sequence                      string
	AppName                       string
	OrgName                       string
	SpaceName                     string
	CredentialsSecretName         string
}
