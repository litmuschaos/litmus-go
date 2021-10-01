package lib

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	awslib "github.com/litmuschaos/litmus-go/pkg/cloud/aws/ec2"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ec2-terminate-by-id/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

var (
	err           error
	inject, abort chan os.Signal
)

//PrepareEC2TerminateByID contains the prepration and injection steps for the experiment
func PrepareEC2TerminateByID(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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

	//get the instance id or list of instance ids
	instanceIDList := strings.Split(experimentsDetails.Ec2InstanceID, ",")
	if len(instanceIDList) == 0 {
		return errors.Errorf("no instance id found to terminate")
	}

	// watching for the abort signal and revert the chaos
	go abortWatcher(experimentsDetails, instanceIDList, chaosDetails)

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(experimentsDetails, instanceIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, instanceIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
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

//injectChaosInSerialMode will inject the ec2 instance termination in serial mode that is one after other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

			log.Infof("[Info]: Target instanceID list, %v", instanceIDList)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on ec2 instance"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			//PowerOff the instance
			for i, id := range instanceIDList {

				//Stopping the EC2 instance
				log.Info("[Chaos]: Stopping the desired EC2 instance")
				if err := awslib.EC2Stop(id, experimentsDetails.Region); err != nil {
					return errors.Errorf("ec2 instance failed to stop, err: %v", err)
				}

				common.SetTargets(id, "injected", "EC2", chaosDetails)

				//Wait for ec2 instance to completely stop
				log.Infof("[Wait]: Wait for EC2 instance '%v' to get in stopped state", id)
				if err := awslib.WaitForEC2Down(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ManagedNodegroup, experimentsDetails.Region, id); err != nil {
					return errors.Errorf("unable to stop the ec2 instance, err: %v", err)
				}

				// run the probes during chaos
				// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
				if len(resultDetails.ProbeDetails) != 0 && i == 0 {
					if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
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
				common.SetTargets(id, "reverted", "EC2", chaosDetails)
			}
			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}
	return nil
}

// injectChaosInParallelMode will inject the ec2 instance termination in parallel mode that is all at once
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
		ChaosStartTimeStamp := time.Now()
		duration := int(time.Since(ChaosStartTimeStamp).Seconds())

		for duration < experimentsDetails.ChaosDuration {

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
				common.SetTargets(id, "injected", "EC2", chaosDetails)
			}

			for _, id := range instanceIDList {
				//Wait for ec2 instance to completely stop
				log.Infof("[Wait]: Wait for EC2 instance '%v' to get in stopped state", id)
				if err := awslib.WaitForEC2Down(experimentsDetails.Timeout, experimentsDetails.Delay, experimentsDetails.ManagedNodegroup, experimentsDetails.Region, id); err != nil {
					return errors.Errorf("unable to stop the ec2 instance, err: %v", err)
				}
				common.SetTargets(id, "reverted", "EC2 Instance ID", chaosDetails)
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
			for _, id := range instanceIDList {
				common.SetTargets(id, "reverted", "EC2", chaosDetails)
			}
			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}
	}
	return nil
}

// watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, instanceIDList []string, chaosDetails *types.ChaosDetails) {

	<-abort

	log.Info("[Abort]: Chaos Revert Started")
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
		common.SetTargets(id, "reverted", "EC2", chaosDetails)
	}
	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
