package lib

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	awslib "github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ec2-terminate/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

//InjectEC2Terminate contains the chaos injection steps for ec2 terminate chaos
func InjectEC2Terminate(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var err error
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Stoping the EC2 instance
	log.Info("[Chaos]: Stoping the desired EC2 instance")
	err = EC2Stop(experimentsDetails)
	if err != nil {
		return errors.Errorf("ec2 instance failed to stop err: %v", err)
	}

	//Wait for ec2 instance to completely stop
	log.Info("[Wait]: Wait for EC2 instance to come in stopped state")
	if err = WaitForEC2Down(experimentsDetails); err != nil {
		return errors.Errorf("unable to stop the ec2 instance err: %v", err)
	}

	//Wait for chaos duration
	log.Infof("[Wait]: Waiting for chaos duration of %vs before starting the instance", experimentsDetails.ChaosDuration)
	time.Sleep(time.Duration(experimentsDetails.ChaosDuration) * time.Second)

	//Starting the EC2 instance
	log.Info("[Chaos]: Starting back the EC2 instance")
	err = EC2Start(experimentsDetails)
	if err != nil {
		return errors.Errorf("ec2 instance failed to start err: %v", err)
	}

	//Wait for ec2 instance to come in running state
	log.Info("[Wait]: Wait for EC2 instance to get in running state")
	if err = WaitForEC2Up(experimentsDetails); err != nil {
		return errors.Errorf("unable to start the ec2 instance err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// EC2Stop will stop an aws ec2 instance
func EC2Stop(experimentsDetails *experimentTypes.ExperimentDetails) error {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(experimentsDetails.Region)},
	}))

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	input := &ec2.StopInstancesInput{
		InstanceIds: []*string{
			aws.String(experimentsDetails.Ec2InstanceID),
		},
	}
	result, err := ec2Svc.StopInstances(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				return errors.Errorf(aerr.Error())
			}
		} else {
			return errors.Errorf(err.Error())
		}
	}

	log.InfoWithValues("Stopping an ec2 instance:", logrus.Fields{
		"CurrentState":  *result.StoppingInstances[0].CurrentState.Name,
		"PreviousState": *result.StoppingInstances[0].PreviousState.Name,
		"InstanceId":    *result.StoppingInstances[0].InstanceId,
	})

	return nil
}

// EC2Start will stop an aws ec2 instance
func EC2Start(experimentsDetails *experimentTypes.ExperimentDetails) error {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(experimentsDetails.Region)},
	}))

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	input := &ec2.StartInstancesInput{
		InstanceIds: []*string{
			aws.String(experimentsDetails.Ec2InstanceID),
		},
	}

	result, err := ec2Svc.StartInstances(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				return errors.Errorf(aerr.Error())
			}
		} else {
			return errors.Errorf(err.Error())
		}
	}

	log.InfoWithValues("Starting ec2 instance:", logrus.Fields{
		"CurrentState":  *result.StartingInstances[0].CurrentState.Name,
		"PreviousState": *result.StartingInstances[0].PreviousState.Name,
		"InstanceId":    *result.StartingInstances[0].InstanceId,
	})

	return nil
}

//WaitForEC2Down will wait for the ec2 instance to get in stopped state
func WaitForEC2Down(experimentsDetails *experimentTypes.ExperimentDetails) error {

	log.Info("[Status]: Checking EC2 instance status")
	err := retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := awslib.GetEC2InstanceStatus(experimentsDetails)
			if err != nil {
				return errors.Errorf("fail to get the instance status")
			}
			if instanceState != "stopped" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in stopped state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
	if err != nil {
		return err
	}
	return nil
}

//WaitForEC2Up will wait for the ec2 instance to get in running state
func WaitForEC2Up(experimentsDetails *experimentTypes.ExperimentDetails) error {

	log.Info("[Status]: Checking EC2 instance status")
	err := retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := awslib.GetEC2InstanceStatus(experimentsDetails)
			if err != nil {
				return errors.Errorf("fail to get the instance status")
			}
			if instanceState != "running" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in running state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
	if err != nil {
		return err
	}
	return nil
}
