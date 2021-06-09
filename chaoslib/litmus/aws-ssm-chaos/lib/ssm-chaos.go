package lib

import (
	"os"
	"strings"
	"time"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/aws-ssm/aws-ssm-chaos/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/ssm"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
)

//InjectChaosInSerialMode will inject the aws ssm chaos in serial mode that is one after other
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, inject chan os.Signal) error {

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

			//Running SSM command on the instance
			for i, ec2ID := range instanceIDList {

				//Sending AWS SSM command
				log.Info("[Chaos]: Starting the ssm command")
				ec2IDList := strings.Fields(ec2ID)
				commandId, err := ssm.SendSSMCommand(experimentsDetails, ec2IDList)
				if err != nil {
					return errors.Errorf("fail to send ssm command, err: %v", err)
				}
				//prepare commands for abort recovery
				experimentsDetails.CommandIDs = append(experimentsDetails.CommandIDs, commandId)

				//wait for the ssm command to get in running state
				log.Info("[Wait]: Waiting for the ssm command to get in InProgress state")
				if err := ssm.WaitForCommandStatus("InProgress", commandId, ec2ID, experimentsDetails.Region, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.Delay); err != nil {
					return errors.Errorf("fail to start ssm command, err: %v", err)
				}

				// run the probes during chaos
				if len(resultDetails.ProbeDetails) != 0 && i == 0 {
					if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
						return err
					}
				}

				//wait for the ssm command to get succeeded in the given chaos duration
				log.Info("[Wait]: Waiting for the ssm command to get completed")
				if err := ssm.WaitForCommandStatus("Success", commandId, ec2ID, experimentsDetails.Region, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.Delay); err != nil {
					return errors.Errorf("fail to send ssm command, err: %v", err)
				}

				//Wait for chaos interval
				log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
				time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)

			}
			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}

	}
	return nil
}

// InjectChaosInParallelMode will inject the aws ssm chaos in parallel mode that is all at once
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, instanceIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, inject chan os.Signal) error {

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

			//Sending AWS SSM command
			log.Info("[Chaos]: Starting the ssm command")
			commandId, err := ssm.SendSSMCommand(experimentsDetails, instanceIDList)
			if err != nil {
				return errors.Errorf("fail to send ssm command, err: %v", err)
			}
			//prepare commands for abort recovery
			experimentsDetails.CommandIDs = append(experimentsDetails.CommandIDs, commandId)

			for _, ec2ID := range instanceIDList {
				//wait for the ssm command to get in running state
				log.Info("[Wait]: Waiting for the ssm command to get in InProgress state")
				if err := ssm.WaitForCommandStatus("InProgress", commandId, ec2ID, experimentsDetails.Region, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.Delay); err != nil {
					return errors.Errorf("fail to start ssm command, err: %v", err)
				}
			}

			// run the probes during chaos
			if len(resultDetails.ProbeDetails) != 0 {
				if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return err
				}
			}

			for _, ec2ID := range instanceIDList {
				//wait for the ssm command to get succeeded in the given chaos duration
				log.Info("[Wait]: Waiting for the ssm command to get completed")
				if err := ssm.WaitForCommandStatus("Success", commandId, ec2ID, experimentsDetails.Region, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.Delay); err != nil {
					return errors.Errorf("fail to send ssm command, err: %v", err)
				}
			}

			//Wait for chaos interval
			log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
			time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)

			duration = int(time.Since(ChaosStartTimeStamp).Seconds())
		}

	}
	return nil
}

// AbortWatcher will be watching for the abort signal and revert the chaos
func AbortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, abort chan os.Signal) {

	<-abort

	log.Info("[Abort]: Chaos Revert Started")
	switch {
	case len(experimentsDetails.CommandIDs) != 0:
		for _, commandId := range experimentsDetails.CommandIDs {
			if err := ssm.CancelCommand(commandId, experimentsDetails.Region); err != nil {
				log.Errorf("[Abort]: fail to cancle command, recovery failed, err: %v", err)
			}
		}
	default:
		log.Info("[Abort]: No command found to cancle")
	}
	if err := ssm.SSMDeleteDocument(experimentsDetails.DocumentName, experimentsDetails.Region); err != nil {
		log.Errorf("fail to delete ssm doc, err: %v", err)
	}
	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
