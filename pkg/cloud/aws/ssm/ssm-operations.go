package ssm

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ssm"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/aws-ssm/aws-ssm-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	ec2 "github.com/litmuschaos/litmus-go/pkg/cloud/aws/ec2"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultSSMDocsDirectory contains path of the ssm docs
	DefaultSSMDocsDirectory = "LitmusChaos-AWS-SSM-Docs.yml"
)

// awsErrHasCode checks if an AWS error has a specific error code
func awsErrHasCode(err error, code string) bool {
	if aerr, ok := err.(awserr.Error); ok {
		return aerr.Code() == code
	}
	return false
}

// SendSSMCommand will create and add the ssm document in aws service monitoring docs.
func SendSSMCommand(experimentsDetails *experimentTypes.ExperimentDetails, ec2InstanceID []string) (string, error) {

	sesh := common.GetAWSSession(experimentsDetails.Region)
	ssmClient := ssm.New(sesh)
	timeout := int64(experimentsDetails.ChaosDuration + 30)
	res, err := ssmClient.SendCommand(&ssm.SendCommandInput{
		DocumentName: aws.String(experimentsDetails.DocumentName),

		Targets: []*ssm.Target{
			{
				Key:    aws.String("InstanceIds"),
				Values: aws.StringSlice(ec2InstanceID),
			},
		},
		Parameters:     getParameters(experimentsDetails),
		TimeoutSeconds: aws.Int64(timeout),
		MaxConcurrency: aws.String("50"),
		MaxErrors:      aws.String("0"),
	})
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to send SSM command: %v", common.CheckAWSError(err).Error()),
			Target:    fmt.Sprintf("{EC2 Instance ID: %v, Region: %v}", ec2InstanceID, experimentsDetails.Region),
		}
	}

	return *res.Command.CommandId, nil
}

// getParameters will return the parameters bases on the doccumentPath
// for custom path no parameter will be sent to the docs
func getParameters(experimentsDetails *experimentTypes.ExperimentDetails) map[string][]*string {

	if experimentsDetails.DocumentPath != DefaultSSMDocsDirectory {
		return nil
	}
	parameter := map[string][]*string{
		"Duration": {
			aws.String(strconv.Itoa(experimentsDetails.ChaosDuration)),
		},
		"CPU": {
			aws.String(strconv.Itoa(experimentsDetails.Cpu)),
		},
		"Workers": {
			aws.String(strconv.Itoa(experimentsDetails.NumberOfWorkers)),
		},
		"Percent": {
			aws.String(strconv.Itoa(experimentsDetails.MemoryPercentage)),
		},
		"InstallDependencies": {
			aws.String(experimentsDetails.InstallDependencies),
		},
	}
	return parameter
}

// WaitForCommandStatus will wait until the ssm command comes in target status
func WaitForCommandStatus(status, commandID, ec2InstanceID, region string, timeout, delay int) error {

	log.Info("[Status]: Checking SSM command status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			commandStatus, err := getSSMCommandStatus(commandID, ec2InstanceID, region)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the SSM command status")
			}
			if commandStatus != status {
				log.Infof("The instance state is %v", commandStatus)
				return cerrors.Error{
					ErrorCode: cerrors.ErrorTypeChaosInject,
					Reason:    fmt.Sprintf("SSM command is not in %v state within timeout", status),
					Target:    fmt.Sprintf("{EC2 Instance ID: %v, Region: %v}", ec2InstanceID, region)}
			}
			log.Infof("The SSM command status is %v", commandStatus)
			return nil
		})
}

// getSSMCommandStatus will create and add the ssm document in aws service monitoring docs.
func getSSMCommandStatus(commandID, ec2InstanceID, region string) (string, error) {

	sesh := common.GetAWSSession(region)
	ssmClient := ssm.New(sesh)

	cmdOutput, err := ssmClient.GetCommandInvocation(&ssm.GetCommandInvocationInput{
		CommandId:  aws.String(commandID),
		InstanceId: aws.String(ec2InstanceID),
	})
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to get SSM command status: %v", common.CheckAWSError(err).Error()),
			Target:    fmt.Sprintf("{Command ID: %v, EC2 Instance ID: %v, Region: %v}", commandID, ec2InstanceID, region),
		}
	}
	return *cmdOutput.Status, nil
}

// CheckInstanceInformation checks if the instance has permission to do SSM API calls,
func CheckInstanceInformation(experimentsDetails *experimentTypes.ExperimentDetails) error {
	var instanceIDList []string
	switch {
	case experimentsDetails.EC2InstanceID != "":
		instanceIDList = strings.Split(experimentsDetails.EC2InstanceID, ",")
	default:
		if err := CheckTargetInstanceStatus(experimentsDetails); err != nil {
			return stacktrace.Propagate(err, "failed to check target instance(s) status")
		}
		instanceIDList = experimentsDetails.TargetInstanceIDList
	}

	sesh := common.GetAWSSession(experimentsDetails.Region)
	ssmClient := ssm.New(sesh)
	var (
		foundInstances   = make(map[string]bool)
		input            = &ssm.DescribeInstanceInformationInput{}
		err              error
		maxRetries       = 5
		maxRetryDuration = time.Second * 30
		startTime        = time.Now()
	)

	for attempt := 0; attempt < maxRetries; attempt++ {
		if time.Since(startTime) > maxRetryDuration {
			break
		}

		err = ssmClient.DescribeInstanceInformationPages(input,
			func(page *ssm.DescribeInstanceInformationOutput, lastPage bool) bool {
				for _, instanceDetails := range page.InstanceInformationList {
					if instanceDetails.InstanceId != nil {
						foundInstances[*instanceDetails.InstanceId] = true
					}
				}
				return true // continue to next page
			})

		if err != nil {
			awsErr := common.CheckAWSError(err)
			if request.IsErrorThrottle(err) ||
				awsErrHasCode(awsErr, "ThrottlingException") ||
				awsErrHasCode(awsErr, "RequestThrottledException") ||
				awsErrHasCode(awsErr, "Throttling") ||
				awsErrHasCode(awsErr, "TooManyRequestsException") ||
				awsErrHasCode(awsErr, "RequestLimitExceeded") {

				// Calculate exponential backoff with jitter
				backoffTime := time.Duration(math.Pow(2, float64(attempt))) * time.Second
				rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
                                 jitter := time.Duration(rnd.Intn(1000)) * time.Millisecond
				sleepTime := backoffTime + jitter

				log.Infof("AWS API rate limit hit, retrying in %v (attempt %d/%d)", sleepTime, attempt+1, maxRetries)
				time.Sleep(sleepTime)
				continue
			}

			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosInject,
				Reason:    fmt.Sprintf("failed to get instance information: %v", awsErr.Error()),
				Target:    fmt.Sprintf("{Region: %v}", experimentsDetails.Region),
			}
		}

		break
	}

	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to get instance information after retries: %v", common.CheckAWSError(err).Error()),
			Target:    fmt.Sprintf("{Region: %v}", experimentsDetails.Region),
		}
	}

	// Validate that each target instance is present.
	for _, ec2ID := range instanceIDList {
		if _, exists := foundInstances[ec2ID]; !exists {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosInject,
				Reason:    fmt.Sprintf("the instance %v might not have suitable permission or IAM attached to it. Run command `aws ssm describe-instance-information` to check for available instances", ec2ID),
				Target:    fmt.Sprintf("{EC2 Instance ID: %v, Region: %v}", ec2ID, experimentsDetails.Region),
			}
		}
	}

	log.Info("[Info]: The target instance have permission to perform SSM API calls")
	return nil
}

// CancelCommand will cancel the ssm command
func CancelCommand(commandIDs, region string) error {
	sesh := common.GetAWSSession(region)
	ssmClient := ssm.New(sesh)
	_, err := ssmClient.CancelCommand(&ssm.CancelCommandInput{
		CommandId: aws.String(commandIDs),
	})
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    fmt.Sprintf("failed to cancel SSM command: %v", common.CheckAWSError(err).Error()),
			Target:    fmt.Sprintf("{SSM Command ID: %v, Region: %v}", commandIDs, region),
		}
	}
	return nil
}

// CheckTargetInstanceStatus will select the target instance which are in running state and
// filtered from the given instance tag and check its status
func CheckTargetInstanceStatus(experimentsDetails *experimentTypes.ExperimentDetails) error {

	instanceIDList, err := ec2.GetInstanceList(experimentsDetails.EC2InstanceTag, experimentsDetails.Region)
	if err != nil {
		return stacktrace.Propagate(err, "failed to get the instance list")
	}
	if len(instanceIDList) == 0 {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeTargetSelection,
			Reason:    "no instance found",
			Target:    fmt.Sprintf("{EC2 Instance Tag: %v, Region: %v}", experimentsDetails.EC2InstanceTag, experimentsDetails.Region),
		}
	}

	for _, id := range instanceIDList {
		instanceState, err := ec2.GetEC2InstanceStatus(id, experimentsDetails.Region)
		if err != nil {
			return stacktrace.Propagate(err, "failed to get the instance status while selecting the target instances")
		}
		if instanceState == "running" {
			experimentsDetails.TargetInstanceIDList = append(experimentsDetails.TargetInstanceIDList, id)
		}
	}

	if len(experimentsDetails.TargetInstanceIDList) == 0 {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    "failed to get any running instance with instance tag",
			Target:    fmt.Sprintf("{EC2 Instance Tag: %v, Region: %v}", experimentsDetails.EC2InstanceTag, experimentsDetails.Region),
		}
	}

	log.InfoWithValues("[Info]: Targeting the running instances filtered from instance tag", logrus.Fields{
		"Total number of instances filtered":   len(instanceIDList),
		"Number of running instances filtered": len(experimentsDetails.TargetInstanceIDList),
	})
	return nil
}
