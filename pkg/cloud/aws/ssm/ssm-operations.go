package ssm

import (
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/aws-ssm/aws-ssm-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	ec2 "github.com/litmuschaos/litmus-go/pkg/cloud/aws/ec2"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	DefaultSSMDocsDirectory = "LitmusChaos-AWS-SSM-Docs.yml"
)

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
		return "", common.CheckAWSError(err)
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

//WaitForCommandStatus will wait until the ssm command comes in target status
func WaitForCommandStatus(status, commandID, EC2InstanceID, region string, timeout, delay int) error {

	log.Info("[Status]: Checking SSM command status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			commandStatus, err := getSSMCommandStatus(commandID, EC2InstanceID, region)
			if err != nil {
				return errors.Errorf("failed to get the ssm command status")
			}
			if commandStatus != status {
				log.Infof("The instance state is %v", commandStatus)
				return errors.Errorf("ssm command is not yet in %v state", status)
			}
			log.Infof("The ssm command status is %v", commandStatus)
			return nil
		})
}

// getSSMCommandStatus will create and add the ssm document in aws service monitoring docs.
func getSSMCommandStatus(commandID, EC2InstanceID, region string) (string, error) {

	sesh := common.GetAWSSession(region)
	ssmClient := ssm.New(sesh)

	cmdOutput, err := ssmClient.GetCommandInvocation(&ssm.GetCommandInvocationInput{
		CommandId:  aws.String(commandID),
		InstanceId: aws.String(EC2InstanceID),
	})
	if err != nil {
		return "", common.CheckAWSError(err)
	}
	return *cmdOutput.Status, nil
}

//CheckInstanceInformation will check if the instance has permission to do smm api calls
func CheckInstanceInformation(experimentsDetails *experimentTypes.ExperimentDetails) error {

	var instanceIDList []string
	switch {
	case experimentsDetails.EC2InstanceID != "":
		instanceIDList = strings.Split(experimentsDetails.EC2InstanceID, ",")
	default:
		if err := CheckTargetInstanceStatus(experimentsDetails); err != nil {
			return err
		}
		instanceIDList = experimentsDetails.TargetInstanceIDList

	}
	sesh := common.GetAWSSession(experimentsDetails.Region)
	ssmClient := ssm.New(sesh)
	for _, ec2ID := range instanceIDList {
		res, err := ssmClient.DescribeInstanceInformation(&ssm.DescribeInstanceInformationInput{})
		if err != nil {
			return common.CheckAWSError(err)
		}
		isInstanceFound := false
		if len(res.InstanceInformationList) != 0 {
			for _, instanceDetails := range res.InstanceInformationList {
				if *instanceDetails.InstanceId == ec2ID {
					isInstanceFound = true
					break
				}
			}
			if !isInstanceFound {
				return errors.Errorf("error: the instance %v might not have suitable permission or iam attached to it. use \"aws ssm describe-instance-information\" to check the available instances", ec2ID)
			}
		}
	}
	log.Info("[Info]: The target instance have permission to perform ssm api calls")
	return nil
}

//CancelCommand will cancel the ssm command
func CancelCommand(commandIDs, region string) error {
	sesh := common.GetAWSSession(region)
	ssmClient := ssm.New(sesh)
	_, err := ssmClient.CancelCommand(&ssm.CancelCommandInput{
		CommandId: aws.String(commandIDs),
	})
	if err != nil {
		return common.CheckAWSError(err)
	}
	return nil
}

//CheckTargetInstanceStatus will select the target instance which are in running state and
// filtered from the given instance tag and check its status
func CheckTargetInstanceStatus(experimentsDetails *experimentTypes.ExperimentDetails) error {

	instanceIDList, err := ec2.GetInstanceList(experimentsDetails.EC2InstanceTag, experimentsDetails.Region)
	if err != nil {
		return err
	}
	if len(instanceIDList) == 0 {
		return errors.Errorf("no instance found with the given tag %v, in region %v", experimentsDetails.EC2InstanceTag, experimentsDetails.Region)
	}

	for _, id := range instanceIDList {
		instanceState, err := ec2.GetEC2InstanceStatus(id, experimentsDetails.Region)
		if err != nil {
			return errors.Errorf("fail to get the instance status while selecting the target instances, err: %v", err)
		}
		if instanceState == "running" {
			experimentsDetails.TargetInstanceIDList = append(experimentsDetails.TargetInstanceIDList, id)
		}
	}

	if len(experimentsDetails.TargetInstanceIDList) == 0 {
		return errors.Errorf("fail to get any running instance having instance tag: %v", experimentsDetails.EC2InstanceTag)
	}

	log.InfoWithValues("[Info]: Targeting the running instances filtered from instance tag", logrus.Fields{
		"Total number of instances filtered":   len(instanceIDList),
		"Number of running instances filtered": len(experimentsDetails.TargetInstanceIDList),
	})
	return nil
}
