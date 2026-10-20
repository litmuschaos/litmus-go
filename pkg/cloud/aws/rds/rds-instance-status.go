package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
)

// GetRDSInstanceStatus will verify and give the rds instance details.
func GetRDSInstanceStatus(instanceIdentifier, region string) (string, error) {

	var err error
	// Load session from shared config
	sess := common.GetAWSSession(region)

	// Create new RDS client
	rdsSvc := rds.New(sess)

	// Call to get detailed information on each instance
	result, err := rdsSvc.DescribeDBInstances(nil)
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("failed to describe the instances: %v", err),
			Target:    fmt.Sprintf("{RDS Instance Identifier: %v, Region: %v}", instanceIdentifier, region),
		}
	}

	for _, instanceDetails := range result.DBInstances {
		if *instanceDetails.DBInstanceIdentifier == instanceIdentifier {
			return *instanceDetails.DBInstanceStatus, nil
		}
	}
	return "", cerrors.Error{
		ErrorCode: cerrors.ErrorTypeStatusChecks,
		Reason:    "failed to get the status of RDS instance",
		Target:    fmt.Sprintf("{RDS Instance Identifier: %v, Region: %v}", instanceIdentifier, region),
	}
}

// InstanceStatusCheckByInstanceIdentifier is used to check the instance status of all the instance under chaos.
func InstanceStatusCheckByInstanceIdentifier(instanceIdentifier, region string) error {

	instanceIdentifierList := stringutils.SplitList(instanceIdentifier)
	if len(instanceIdentifierList) == 0 {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    "no instance identifier provided to stop",
			Target:    fmt.Sprintf("{RDS Instance Identifier: %v, Region: %v}", instanceIdentifier, region),
		}
	}
	log.Infof("[Info]: The instances under chaos(IUC) are: %v", instanceIdentifierList)
	return InstanceStatusCheck(instanceIdentifierList, region)
}

// InstanceStatusCheck is used to check the instance status of the instances.
func InstanceStatusCheck(instanceIdentifierList []string, region string) error {

	for _, id := range instanceIdentifierList {
		instanceState, err := GetRDSInstanceStatus(id, region)
		if err != nil {
			return err
		}
		if instanceState != "available" {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeStatusChecks,
				Reason:    fmt.Sprintf("rds instance is not in available state, current state: %v", instanceState),
				Target:    fmt.Sprintf("{RDS Instance Identifier: %v, Region: %v}", id, region),
			}
		}
	}
	return nil
}
