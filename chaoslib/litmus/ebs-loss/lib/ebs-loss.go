package lib

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	ebs "github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/ebs-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

//InjectEBSLoss contains the chaos injection steps for ebs loss
func InjectEBSLoss(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var err error
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Detaching the ebs volume from the instance
	log.Info("[Chaos]: Detaching the EBS volume from the instance")
	err = EBSVolumeDetach(experimentsDetails)
	if err != nil {
		return errors.Errorf("ebs detachment failed err: %v", err)
	}

	//Wait for chaos duration
	log.Infof("[Wait]: Waiting for the chaos duration of %vs", experimentsDetails.ChaosDuration)
	time.Sleep(time.Duration(experimentsDetails.ChaosDuration) * time.Second)

	//Getting the Ebs status
	EBSStatus, err := ebs.GetEBSStatus(experimentsDetails)
	if err != nil {
		return errors.Errorf("fail to get the ebs status err: %v", err)
	}

	if EBSStatus != "attached" {
		//Attaching the ebs volume from the instance
		log.Info("[Chaos]: Attaching the EBS volume from the instance")
		err = EBSVolumeAttach(experimentsDetails)
		if err != nil {
			return errors.Errorf("ebs attachment failed err: %v", err)
		}
	} else {
		log.Info("[Skip]: The EBS volume is already attached")
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// EBSVolumeDetach will detach the ebs vol from ec2 node
func EBSVolumeDetach(experimentsDetails *experimentTypes.ExperimentDetails) error {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(experimentsDetails.Region)},
	}))

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	input := &ec2.DetachVolumeInput{
		VolumeId: aws.String(experimentsDetails.EBSVolumeID),
	}

	result, err := ec2Svc.DetachVolume(input)
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

	log.InfoWithValues("Detaching ebs having:", logrus.Fields{
		"VolumeId":   *result.VolumeId,
		"State":      *result.State,
		"Device":     *result.Device,
		"InstanceId": *result.InstanceId,
	})

	return nil
}

// EBSVolumeAttach will detach the ebs vol from ec2 node
func EBSVolumeAttach(experimentsDetails *experimentTypes.ExperimentDetails) error {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(experimentsDetails.Region)},
	}))

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	//Attaching the ebs volume after chaos
	input := &ec2.AttachVolumeInput{
		Device:     aws.String(experimentsDetails.DeviceName),
		InstanceId: aws.String(experimentsDetails.Ec2InstanceID),
		VolumeId:   aws.String(experimentsDetails.EBSVolumeID),
	}

	result, err := ec2Svc.AttachVolume(input)
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

	log.InfoWithValues("Attaching ebs having:", logrus.Fields{
		"VolumeId":   *result.VolumeId,
		"State":      *result.State,
		"Device":     *result.Device,
		"InstanceId": *result.InstanceId,
	})

	return nil
}
