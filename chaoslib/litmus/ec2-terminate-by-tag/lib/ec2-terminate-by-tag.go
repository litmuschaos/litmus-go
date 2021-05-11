package lib

import (
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
	"github.com/sirupsen/logrus"
)

var inject, abort chan os.Signal

//PrepareEC2TerminateByTag contains the prepration and injection steps for the experiment
func PrepareEC2TerminateByTag(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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

	instanceIDList := CalculateInstanceAffPerc(experimentsDetails.InstanceAffectedPerc, experimentsDetails.TargetInstanceIDList)
	log.Infof("[Chaos]:Number of Instance targeted: %v", len(instanceIDList))

	// watching for the abort signal and revert the chaos
	go abortWatcher(experimentsDetails, clients, resultDetails, chaosDetails, eventsDetails)

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err := injectChaosInSerialMode(experimentsDetails, instanceIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	case "parallel":
		if err := injectChaosInParallelMode(experimentsDetails, instanceIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//injectChaosInSerialMode will inject the ce2 instance termination in serial mode that is one after other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now().Unix()

	loop:
		for {

			log.Infof("[Info]: Target instanceID list, %v", instanceIDList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on ec2 instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			//PowerOff the instance
			for _, id := range instanceIDList {

				//Stopping the EC2 instance
				log.Info("[Chaos]: Stopping the desired EC2 instance")
				if err := awslib.EC2Stop(id, experimentsDetails.Region); err != nil {
					return errors.Errorf("ec2 instance failed to stop, err: %v", err)
				}

				//Wait for ec2 instance to completely stop
				log.Infof("[Wait]: Wait for EC2 instance '%v' to get in stopped state", id)
				if err := awslib.WaitForEC2Down(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ManagedNodegroup, experimentsDetails.Region, id); err != nil {
					return errors.Errorf("unable to stop the ec2 instance, err: %v", err)
				}

				// run the probes during chaos
				if len(resultDetails.ProbeDetails) != 0 {
					if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
						return err
					}
				}

				//Wait for chaos interval
				log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
				time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)

				//Starting the EC2 instance
				if experimentsDetails.ManagedNodegroup != "enable" {
					log.Info("[Chaos]: Starting back the EC2 instance")
					if err := awslib.EC2Start(id, experimentsDetails.Region); err != nil {
						return errors.Errorf("ec2 instance failed to start, err: %v", err)
					}

					//Wait for ec2 instance to get in running state
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
	}
	return nil
}

// injectChaosInParallelMode will inject the ce2 instance termination in parallel mode that is all at once
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now().Unix()

	loop:
		for {

			log.Infof("[Info]: Target instanceID list, %v", instanceIDList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on ec2 instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			//PowerOff the instance
			for _, id := range instanceIDList {
				//Stopping the EC2 instance
				log.Info("[Chaos]: Stopping the desired EC2 instance")
				if err := awslib.EC2Stop(id, experimentsDetails.Region); err != nil {
					return errors.Errorf("ec2 instance failed to stop, err: %v", err)
				}
			}

			for _, id := range instanceIDList {
				//Wait for ec2 instance to completely stop
				log.Infof("[Wait]: Wait for EC2 instance '%v' to get in stopped state", id)
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
			log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
			time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)

			//Starting the EC2 instance
			if experimentsDetails.ManagedNodegroup != "enable" {

				for _, id := range instanceIDList {
					log.Info("[Chaos]: Starting back the EC2 instance")
					if err := awslib.EC2Start(id, experimentsDetails.Region); err != nil {
						return errors.Errorf("ec2 instance failed to start, err: %v", err)
					}
				}

				for _, id := range instanceIDList {
					//Wait for ec2 instance to get in running state
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

//PostChaosInstanceStatusCheck is used to check the instance status of the instances under chaos post chaos.
func PostChaosInstanceStatusCheck(targetInstanceIDList []string, region string) error {

	for _, id := range targetInstanceIDList {
		instanceState, err := awslib.GetEC2InstanceStatus(id, region)
		if err != nil {
			return err
		}
		if instanceState != "running" {
			return errors.Errorf("failed to get the ec2 instance '%v' in running sate, current state: %v", id, instanceState)
		}
	}
	return nil
}

//SetTargetInstance will select the target instance which are in running state and filtered from the given instance tag
func SetTargetInstance(experimentsDetails *experimentTypes.ExperimentDetails) error {

	instanceIDList, err := awslib.GetInstanceList(experimentsDetails.InstanceTag, experimentsDetails.Region)
	if err != nil {
		return err
	}
	if len(instanceIDList) == 0 {
		return errors.Errorf("no instance found with the given tag %v, in region %v", experimentsDetails.InstanceTag, experimentsDetails.Region)
	}

	for _, id := range instanceIDList {
		instanceState, err := awslib.GetEC2InstanceStatus(id, experimentsDetails.Region)
		if err != nil {
			return errors.Errorf("fail to get the instance status while selecting the target instances, err: %v", err)
		}
		if instanceState == "running" {
			experimentsDetails.TargetInstanceIDList = append(experimentsDetails.TargetInstanceIDList, id)
		}
	}

	if len(experimentsDetails.TargetInstanceIDList) == 0 {
		return errors.Errorf("fail to get any running instance having instance tag: %v", experimentsDetails.InstanceTag)
	}

	log.InfoWithValues("[Info]: Targeting the running instances filtered from instance tag", logrus.Fields{
		"Total number of instances filtered":   len(instanceIDList),
		"Number of running instances filtered": len(experimentsDetails.TargetInstanceIDList),
	})
	return nil
}

// watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails) {

	<-abort

	log.Info("[Abort]: Chaos Experiment Abortion started because of terminated signal received")

	instanceIDList, err := awslib.GetInstanceList(experimentsDetails.InstanceTag, experimentsDetails.Region)
	if err != nil {
		log.Errorf("fail to get instance list when abort signal is received", err)
	}
	for _, id := range instanceIDList {
		instanceState, err := awslib.GetEC2InstanceStatus(id, experimentsDetails.Region)
		if err != nil {
			log.Errorf("fail to get instance status when an abort signal is received,err :%v", err)
		}
		if instanceState != "running" && experimentsDetails.ManagedNodegroup != "enable" {

			log.Info("[Abort]: Waiting for the EC2 instance to get down")
			if err := awslib.WaitForEC2Down(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ManagedNodegroup, experimentsDetails.Region, id); err != nil {
				log.Errorf("unable to wait till stop of the instance, err: %v", err)
			}

			log.Info("[Abort]: Starting EC2 instance as abort signal received")
			err := awslib.EC2Start(id, experimentsDetails.Region)
			if err != nil {
				log.Errorf("ec2 instance failed to start when an abort signal is received, err: %v", err)
			}
		}
	}
	os.Exit(1)
}
