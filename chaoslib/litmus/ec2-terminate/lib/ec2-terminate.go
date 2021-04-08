package lib

import (
	"math/rand"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	awslib "github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ec2-terminate/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

//PrepareEC2Terminate contains the prepration and injection steps for the experiment
func PrepareEC2Terminate(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var err error
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.Ec2InstanceID == "" && experimentsDetails.InstanceTag == "" {
		return errors.Errorf("Please provide one of the Instance ID or Instance Tag")
	}
	instanceIDList, err := GetInstanceList(experimentsDetails.Ec2InstanceID, experimentsDetails.InstanceTag, experimentsDetails.Region)
	if err != nil {
		return err
	}
	if len(instanceIDList) == 0 {
		return errors.Errorf("fail to extract the instance id")
	}
	if experimentsDetails.Ec2InstanceID == "" {
		instanceIDList = CalculateInstanceAffPerc(experimentsDetails.InstanceAffectedPerc, instanceIDList)
	}
	log.Infof("[Chaos]:Number of Instance targeted: %v", len(instanceIDList))

	if strings.ToLower(experimentsDetails.Sequence) == "serial" {
		if err = InjectChaosInSerialMode(experimentsDetails, instanceIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	} else {
		if err = InjectChaosInParallelMode(experimentsDetails, instanceIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	}
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//InjectChaosInSerialMode will inject the ce2 instance termination in serial mode that is one after other
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now().Unix()

loop:
	for {

		log.Infof("Target instanceID list, %v", instanceIDList)

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on ec2 instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		//PowerOff the instance
		for _, id := range instanceIDList {

			experimentsDetails.Ec2InstanceID = id
			//Stoping the EC2 instance
			log.Info("[Chaos]: Stoping the desired EC2 instance")
			err := EC2Stop(id, experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("ec2 instance failed to stop, err: %v", err)
			}

			//Wait for ec2 instance to completely stop
			log.Infof("[Wait]: Wait for EC2 instance '%v' to come in stopped state", id)
			if err := WaitForEC2Down(experimentsDetails); err != nil {
				return errors.Errorf("unable to stop the ec2 instance, err: %v", err)
			}

			// run the probes during chaos
			if len(resultDetails.ProbeDetails) != 0 {
				if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return err
				}
			}

			//Wait for chaos interval
			log.Infof("[Wait]: Waiting for chaos interval of %vs before starting the instance", experimentsDetails.ChaosInterval)
			time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)

			//Starting the EC2 instance
			if experimentsDetails.ManagedNodegroup != "enable" {
				log.Info("[Chaos]: Starting back the EC2 instance")
				err = EC2Start(id, experimentsDetails.Region)
				if err != nil {
					return errors.Errorf("ec2 instance failed to start, err: %v", err)
				}

				//Wait for ec2 instance to come in running state
				log.Infof("[Wait]: Wait for EC2 instance '%v' to get in running state", id)
				if err := WaitForEC2Up(experimentsDetails); err != nil {
					return errors.Errorf("unable to start the ec2 instance, err: %v", err)
				}
			}

			//ChaosCurrentTimeStamp contains the current timestamp
			ChaosCurrentTimeStamp := time.Now().Unix()

			//ChaosDiffTimeStamp contains the difference of current timestamp and start timestamp
			//It will helpful to track the total chaos duration
			chaosDiffTimeStamp := ChaosCurrentTimeStamp - ChaosStartTimeStamp

			if int(chaosDiffTimeStamp) >= experimentsDetails.ChaosDuration {
				log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
				break loop
			}

		}
	}

	return nil
}

// InjectChaosInParallelMode will inject the ce2 instance termination in parallel mode that is all at once
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now().Unix()

loop:
	for {

		log.Infof("Target instanceID list, %v", instanceIDList)

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on ec2 instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		//PowerOff the instance
		for _, id := range instanceIDList {
			//Stoping the EC2 instance
			log.Info("[Chaos]: Stoping the desired EC2 instance")
			err := EC2Stop(id, experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("ec2 instance failed to stop, err: %v", err)
			}
		}

		for _, id := range instanceIDList {
			//Wait for ec2 instance to completely stop
			log.Infof("[Wait]: Wait for EC2 instance '%v' to come in stopped state", id)
			experimentsDetails.Ec2InstanceID = id
			if err := WaitForEC2Down(experimentsDetails); err != nil {
				return errors.Errorf("unable to stop the ec2 instance, err: %v", err)
			}
		}

		// run the probes during chaos
		if len(resultDetails.ProbeDetails) != 0 {
			if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
				return err
			}
		}

		//Wait for chaos interval
		log.Infof("[Wait]: Waiting for chaos interval of %vs before starting the instance", experimentsDetails.ChaosInterval)
		time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)

		//Starting the EC2 instance
		if experimentsDetails.ManagedNodegroup != "enable" {

			for _, id := range instanceIDList {
				log.Info("[Chaos]: Starting back the EC2 instance")
				err := EC2Start(id, experimentsDetails.Region)
				if err != nil {
					return errors.Errorf("ec2 instance failed to start, err: %v", err)
				}
			}

			for _, id := range instanceIDList {
				//Wait for ec2 instance to come in running state
				log.Infof("[Wait]: Wait for EC2 instance '%v' to get in running state", id)
				experimentsDetails.Ec2InstanceID = id
				if err := WaitForEC2Up(experimentsDetails); err != nil {
					return errors.Errorf("unable to start the ec2 instance, err: %v", err)
				}
			}
		}

		//ChaosCurrentTimeStamp contains the current timestamp
		ChaosCurrentTimeStamp := time.Now().Unix()

		//ChaosDiffTimeStamp contains the difference of current timestamp and start timestamp
		//It will helpful to track the total chaos duration
		chaosDiffTimeStamp := ChaosCurrentTimeStamp - ChaosStartTimeStamp

		if int(chaosDiffTimeStamp) >= experimentsDetails.ChaosDuration {
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			break loop
		}

	}

	return nil
}

//GetInstanceList will filter out the target instance under chaos using tag filters or the instance list provided.
func GetInstanceList(instanceID, instanceTag, region string) ([]string, error) {

	var instanceList []string
	if instanceID != "" && instanceTag != "" {
		log.Info("[Info]: Both instance id and instance tag are provided so the preferance will be given to the instance id")
		instanceTag = ""
	}
	switch instanceTag {
	case "":
		//get the instance id or list of instance ids
		instanceList := strings.Split(instanceID, ",")
		return instanceList, nil

	default:
		instanceTag := strings.Split(instanceTag, ":")
		sess := session.Must(session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigEnable,
			Config:            aws.Config{Region: aws.String(region)},
		}))

		params := &ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				&ec2.Filter{
					Name: aws.String("tag:" + instanceTag[0]),
					Values: []*string{
						aws.String(instanceTag[1]),
					},
				},
			},
		}
		ec2Svc := ec2.New(sess)
		res, err := ec2Svc.DescribeInstances(params)
		if err != nil {
			return nil, errors.Errorf("fail to list the insances, err: %v", err.Error())
		}

		for _, reservationDetails := range res.Reservations {
			for _, i := range reservationDetails.Instances {
				for _, t := range i.Tags {
					if *t.Key == instanceTag[0] {
						instanceList = append(instanceList, *i.InstanceId)
						break
					}
				}
			}
		}
	}
	return instanceList, nil
}

//CalculateInstanceAffPerc will calculate the target instance ids according to the instance affected percentage provided.
func CalculateInstanceAffPerc(podAffPerc int, instanceList []string) []string {

	var newIDList []string
	newInstanceListLength := math.Maximum(1, math.Adjustment(podAffPerc, len(instanceList)))
	rand.Seed(time.Now().UnixNano())

	// it will generate the random instanceList
	// it starts from the random index and choose requirement no of instanceID next to that index in a circular way.
	index := rand.Intn(len(instanceList))
	for i := 0; i < newInstanceListLength; i++ {
		newIDList = append(newIDList, instanceList[index])
		index = (index + 1) % len(instanceList)
	}
	return newIDList
}

// EC2Stop will stop an aws ec2 instance
func EC2Stop(instanceID, region string) error {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(region)},
	}))

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	input := &ec2.StopInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
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
func EC2Start(instanceID, region string) error {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(region)},
	}))

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	input := &ec2.StartInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
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

			instanceState, err := awslib.GetEC2InstanceStatus(experimentsDetails.Ec2InstanceID, experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if (experimentsDetails.ManagedNodegroup != "enable" && instanceState != "stopped") || (experimentsDetails.ManagedNodegroup == "enable" && instanceState != "terminated") {
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

			instanceState, err := awslib.GetEC2InstanceStatus(experimentsDetails.Ec2InstanceID, experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
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

//InstanceStatusCheck is used to check the instance status of all the instance under chaos.
func InstanceStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails) error {

	instanceIDList, err := GetInstanceList(experimentsDetails.Ec2InstanceID, experimentsDetails.InstanceTag, experimentsDetails.Region)
	if err != nil {
		return err
	}
	log.Infof("[Info]: The instances under chaos(IUC) are: %v", instanceIDList)
	for _, id := range instanceIDList {
		instanceState, err := awslib.GetEC2InstanceStatus(id, experimentsDetails.Region)
		if err != nil {
			return err
		}
		if instanceState != "running" {
			return errors.Errorf("failed to get the ec2 instance '%v' status as running", id)
		}
	}
	return nil
}
