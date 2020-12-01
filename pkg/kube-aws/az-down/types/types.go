package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ADD THE ATTRIBUTES OF YOUR CHOICE HERE
// FEW MENDATORY ATTRIBUTES ARE ADDED BY DEFAULT 

// ExperimentDetails is for collecting all the experiment-related details
// TODO make sure these are all relevant
type ExperimentDetails struct {
	ExperimentName      string
	EngineName          string
	ChaosDuration       int
	ChaosInterval       int
	RampTime            int
	ChaosLib            string
	AppNS               string
	AppLabel            string
	AppKind             string
	ChaosUID            clientTypes.UID
	InstanceID          string
	ChaosNamespace      string
	ChaosPodName        string
	Timeout             int
	Delay               int
	AwsRegion           string
	NumberOfAZs         int
	ClusterIdentifier   string
	NodeIdentifiers     string
	NumberNodesToTarget int
}

type InstanceDetails struct {
	Name        string
	ID          string
	AZ          string
	SecGroupIds []string
}
