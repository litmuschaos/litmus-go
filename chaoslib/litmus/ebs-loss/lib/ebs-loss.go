package lib

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	ebs "github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	err           error
	inject, abort chan os.Signal
)

//InjectEBSLoss contains the chaos injection steps for ebs loss
func InjectEBSLoss(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	// watching for the abort signal and revert the chaos
	go abortWatcher(experimentsDetails, clients, resultDetails, chaosDetails, eventsDetails)

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:

		//Detaching the ebs volume from the instance
		log.Info("[Chaos]: Detaching the EBS volume from the instance")
		err = EBSVolumeDetach(experimentsDetails)
		if err != nil {
			return errors.Errorf("ebs detachment failed, err: %v", err)
		}

		//Wait for ebs volume detachment
		log.Info("[Wait]: Wait for EBS volume detachment")
		if err = WaitForVolumeDetachment(experimentsDetails); err != nil {
			return errors.Errorf("unable to detach the ebs volume to the ec2 instance, err: %v", err)
		}

		// run the probes during chaos
		if len(resultDetails.ProbeDetails) != 0 {
			if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
				return err
			}
		}

		//Wait for chaos duration
		log.Infof("[Wait]: Waiting for the chaos duration of %vs", experimentsDetails.ChaosDuration)
		time.Sleep(time.Duration(experimentsDetails.ChaosDuration) * time.Second)

		//Getting the EBS volume attachment status
		EBSStatus, err := ebs.GetEBSStatus(experimentsDetails)
		if err != nil {
			return errors.Errorf("failed to get the ebs status, err: %v", err)
		}

		if EBSStatus != "attached" {
			//Attaching the ebs volume from the instance
			log.Info("[Chaos]: Attaching the EBS volume from the instance")
			err = EBSVolumeAttach(experimentsDetails)
			if err != nil {
				return errors.Errorf("ebs attachment failed, err: %v", err)
			}

			//Wait for ebs volume attachment
			log.Info("[Wait]: Wait for EBS volume attachment")
			if err = WaitForVolumeAttachment(experimentsDetails); err != nil {
				return errors.Errorf("unable to attach the ebs volume to the ec2 instance, err: %v", err)
			}
		} else {
			log.Info("[Skip]: The EBS volume is already attached")
		}
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

// WaitForVolumeDetachment will wait the ebs volume to completely detach
func WaitForVolumeDetachment(experimentsDetails *experimentTypes.ExperimentDetails) error {

	log.Info("[Status]: Checking ebs volume status for detachment")
	err := retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := ebs.GetEBSStatus(experimentsDetails)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if instanceState != "detached" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in detached state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
	if err != nil {
		return err
	}
	return nil
}

// WaitForVolumeAttachment will wait for the ebs volume to get attached on ec2 instance
func WaitForVolumeAttachment(experimentsDetails *experimentTypes.ExperimentDetails) error {

	log.Info("[Status]: Checking ebs volume status for attachment")
	err := retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := ebs.GetEBSStatus(experimentsDetails)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if instanceState != "attached" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in attached state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
	if err != nil {
		return err
	}
	return nil
}

// watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails) {

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM)

loop:
	for {
		select {
		case <-signChan:

			log.Info("[Chaos]: Chaos Experiment Abortion started because of terminated signal received")
			//Getting the EBS volume attachment status
			EBSStatus, err := ebs.GetEBSStatus(experimentsDetails)
			if err != nil {
				log.Errorf("failed to get the ebs status when an abort signal is received, err: %v", err)
			}

			if EBSStatus != "attached" {
				//Attaching the ebs volume from the instance
				log.Info("[Chaos]: Attaching the EBS volume from the instance")
				err = EBSVolumeAttach(experimentsDetails)
				if err != nil {
					log.Errorf("ebs attachment failed when an abort signal is received, err: %v", err)
				}
			}
			break loop
		}
	}
}
