package lib

import (
	"math/rand"
	"strings"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	awslib "github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ec2-terminate-by-tag/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

//PrepareEC2TerminateByTag contains the prepration and injection steps for the experiment
func PrepareEC2TerminateByTag(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var err error
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	instanceIDList, err := awslib.GetInstanceList(experimentsDetails.InstanceTag, experimentsDetails.Region)
	if err != nil {
		return err
	}
	if len(instanceIDList) == 0 {
		return errors.Errorf("fail to extract the instance id")
	}
	instanceIDList = CalculateInstanceAffPerc(experimentsDetails.InstanceAffectedPerc, instanceIDList)
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

			//Stoping the EC2 instance
			log.Info("[Chaos]: Stoping the desired EC2 instance")
			err := awslib.EC2Stop(id, experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("ec2 instance failed to stop, err: %v", err)
			}

			common.SetTargets(id, "injected", "EC2 Instance ID", chaosDetails)

			//Wait for ec2 instance to completely stop
			log.Infof("[Wait]: Wait for EC2 instance '%v' to come in stopped state", id)
			if err := awslib.WaitForEC2Down(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ManagedNodegroup, experimentsDetails.Region, id); err != nil {
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
				err = awslib.EC2Start(id, experimentsDetails.Region)
				if err != nil {
					return errors.Errorf("ec2 instance failed to start, err: %v", err)
				}

				common.SetTargets(id, "reverted", "EC2 Instance ID", chaosDetails)

				//Wait for ec2 instance to come in running state
				log.Infof("[Wait]: Wait for EC2 instance '%v' to get in running state", id)
				if err := awslib.WaitForEC2Up(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ManagedNodegroup, experimentsDetails.Region, id); err != nil {
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
			err := awslib.EC2Stop(id, experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("ec2 instance failed to stop, err: %v", err)
			}
			common.SetTargets(id, "injected", "EC2 Instance ID", chaosDetails)
		}

		for _, id := range instanceIDList {
			//Wait for ec2 instance to completely stop
			log.Infof("[Wait]: Wait for EC2 instance '%v' to come in stopped state", id)
			if err := awslib.WaitForEC2Down(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ManagedNodegroup, experimentsDetails.Region, id); err != nil {
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
				err := awslib.EC2Start(id, experimentsDetails.Region)
				if err != nil {
					return errors.Errorf("ec2 instance failed to start, err: %v", err)
				}
				common.SetTargets(id, "reverted", "EC2 Instance ID", chaosDetails)
			}

			for _, id := range instanceIDList {
				//Wait for ec2 instance to come in running state
				log.Infof("[Wait]: Wait for EC2 instance '%v' to get in running state", id)
				if err := awslib.WaitForEC2Up(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ManagedNodegroup, experimentsDetails.Region, id); err != nil {
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

//CalculateInstanceAffPerc will calculate the target instance ids according to the instance affected percentage provided.
func CalculateInstanceAffPerc(InstanceAffPerc int, instanceList []string) []string {

	var newIDList []string
	newInstanceListLength := math.Maximum(1, math.Adjustment(InstanceAffPerc, len(instanceList)))
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

//InstanceStatusCheckByTag is used to check the instance status of all the instance under chaos.
func InstanceStatusCheckByTag(instanceTag, region string) error {

	instanceIDList, err := awslib.GetInstanceList(instanceTag, region)
	if err != nil {
		return err
	}
	log.Infof("[Info]: The instances under chaos(IUC) are: %v", instanceIDList)
	for _, id := range instanceIDList {
		instanceState, err := awslib.GetEC2InstanceStatus(id, region)
		if err != nil {
			return err
		}
		if instanceState != "running" {
			return errors.Errorf("failed to get the ec2 instance '%v' status as running", id)
		}
	}
	return nil
}
