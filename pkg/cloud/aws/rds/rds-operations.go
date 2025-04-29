package aws

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
)

// RDSInstanceStop will stop an aws rds instance
func RDSInstanceStop(identifier, region string) error {

	// Load session from shared config
	sess := common.GetAWSSession(region)

	// Create new RDS client
	rdsSvc := rds.New(sess)

	input := &rds.StopDBInstanceInput{
		DBInstanceIdentifier: aws.String(identifier),
	}
	result, err := rdsSvc.StopDBInstance(input)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to stop RDS instance: %v", common.CheckAWSError(err).Error()),
			Target:    fmt.Sprintf("{RDS Instance Identifier: %v, Region: %v}", identifier, region),
		}
	}

	log.InfoWithValues("Stopping RDS instance:", logrus.Fields{
		"DBInstanceStatus":     *result.DBInstance.DBInstanceStatus,
		"DBInstanceIdentifier": *result.DBInstance.DBInstanceIdentifier,
	})

	return nil
}

// RDSInstanceStart will start an aws rds instance
func RDSInstanceStart(identifier, region string) error {

	// Load session from shared config
	sess := common.GetAWSSession(region)

	// Create new RDS client
	rdsSvc := rds.New(sess)

	input := &rds.StartDBInstanceInput{
		DBInstanceIdentifier: aws.String(identifier),
	}
	result, err := rdsSvc.StartDBInstance(input)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to start RDS instance: %v", common.CheckAWSError(err).Error()),
			Target:    fmt.Sprintf("{RDS Instance Identifier: %v, Region: %v}", identifier, region),
		}
	}

	log.InfoWithValues("Starting RDS instance:", logrus.Fields{
		"DBInstanceStatus":     *result.DBInstance.DBInstanceStatus,
		"DBInstanceIdentifier": *result.DBInstance.DBInstanceIdentifier,
	})

	return nil
}

// WaitForRDSInstanceDown will wait for the rds instance to get in stopped state
func WaitForRDSInstanceDown(timeout, delay int, region, identifier string) error {

	log.Info("[Status]: Checking RDS instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetRDSInstanceStatus(identifier, region)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the status of RDS instance")
			}
			if instanceState != "stopped" {
				log.Infof("The instance state is %v", instanceState)
				return cerrors.Error{
					ErrorCode: cerrors.ErrorTypeStatusChecks,
					Reason:    fmt.Sprintf("RDS instance is not in stopped state"),
					Target:    fmt.Sprintf("{RDS Instance Identifier: %v, Region: %v}", identifier, region),
				}
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
}

// WaitForRDSInstanceUp will wait for the rds instance to get in available state
func WaitForRDSInstanceUp(timeout, delay int, region, identifier string) error {

	log.Info("[Status]: Checking RDS instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetRDSInstanceStatus(identifier, region)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the status of RDS instance")
			}
			if instanceState != "available" {
				log.Infof("The instance state is %v", instanceState)
				return cerrors.Error{
					ErrorCode: cerrors.ErrorTypeStatusChecks,
					Reason:    fmt.Sprintf("RDS instance is not in available state"),
					Target:    fmt.Sprintf("{RDS Instance Identifier: %v, Region: %v}", identifier, region),
				}
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
}
