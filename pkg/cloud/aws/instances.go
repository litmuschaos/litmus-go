package aws

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/ec2"
	azDown "github.com/litmuschaos/litmus-go/pkg/kube-aws/az-down/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"strings"
)

// returns all instances by name, id and az for region that cluster identifier
func GetAllInstancesInRegion(ec2Svc *ec2.EC2) ([]*azDown.InstanceDetails, error) {

	instanceData, err := ec2Svc.DescribeInstances(&ec2.DescribeInstancesInput{})

	if err != nil {
		log.Error(fmt.Sprintf("Could not list instances in region %s", err.Error()))
	}

	reservations := instanceData.Reservations

	instances := make([]*azDown.InstanceDetails, len(reservations))
	for i, res := range reservations {
		instance := azDown.InstanceDetails{
			Name: *res.Instances[0].InstanceId,
			ID:   *res.Instances[0].Placement.AvailabilityZone,
			AZ:   *res.Instances[0].Placement.AvailabilityZone,
		}
		instances[i] = &instance
	}

	return instances, nil
}

// returns a list of all instances whose name contains a specified cluster name identifier
func GetClusterInstancesInRegion(ec2Svc *ec2.EC2, details *azDown.ExperimentDetails) ([]*azDown.InstanceDetails, error) {
	allInstances, err := GetAllInstancesInRegion(ec2Svc)
	if err != nil {
		return nil, err
	}

	clusterInstances := []*azDown.InstanceDetails{}
	for _, instance := range allInstances {
		name := instance.Name
		if strings.Contains(name, details.ClusterIdentifier) {
			clusterInstances = append(clusterInstances, instance)
		}
	}
	return clusterInstances, nil
}
