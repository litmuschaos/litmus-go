package types

import (
	"fmt"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// ADD THE ATTRIBUTES OF YOUR CHOICE HERE
// FEW MENDATORY ATTRIBUTES ARE ADDED BY DEFAULT

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName    string
	EngineName        string
	ChaosDuration     int
	ChaosInterval     int
	RampTime          int
	ChaosLib          string
	AppNS             string
	AppLabel          string
	AppKind           string
	ChaosUID          clientTypes.UID
	InstanceID        string
	ChaosNamespace    string
	ChaosPodName      string
	Timeout           int
	Delay             int
	AwsRegion         string
	ClusterIdentifier string
	DryRun            bool
}

type InstanceDetails struct {
	Name string
	ID   string
	AZ   string
}

type Subnet struct {
	Id                      string
	VpcId                   string
	NetworkAclAssociationId string
	NetworkAclId            string
	DummyAclId              string
}

type AzDetails struct {
	AzName  string
	Subnets []*Subnet
}

func (az AzDetails) Print() {
	for _, subnet := range az.Subnets {
		fmt.Sprintf("Id: %s VpcId: %s NetworkAclAssociationId: %s NetworkAclId: %s DummyAclId: %s", subnet.Id, subnet.VpcId, subnet.NetworkAclAssociationId, subnet.NetworkAclId, subnet.DummyAclId)
	}
}

func (inst InstanceDetails) Print() {
	fmt.Sprintf("Instance:\nID: %s\t, AZ: %s\t, Name: %s\t", inst.ID, inst.AZ, inst.Name)
}
