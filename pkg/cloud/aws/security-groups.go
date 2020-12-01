package aws

import (
	"github.com/aws/aws-sdk-go/service/ec2"
)

func GetSecurityGroupsByIds(ec2Svc *ec2.EC2, secGroupIds []*string) (*ec2.DescribeSecurityGroupsOutput, error) {
	input := ec2.DescribeSecurityGroupsInput{GroupIds: secGroupIds}
	return ec2Svc.DescribeSecurityGroups(&input)
}

func CreateNewSecurityGroup(ec2Svc *ec2.EC2, input *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	return ec2Svc.CreateSecurityGroup(input)
}

func AssignSecurityGroupToInstance(ec2Svc *ec2.EC2) {
	// TODO use aws client to assign security group to instance
}

func RemoveSecurityGroupFromInstance(ec2Svc *ec2.EC2) {
	// TODO use aws client to remove security group from instance
}

func RemoveSecurityGroupFromInstances(ec2Svc *ec2.EC2) {
	// TODO use aws client to remove security group from instance
}
