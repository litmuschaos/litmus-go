package lib

import (
	"strconv"
	"strings"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/machine/common/messages"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/os/process-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

// PrepareProcessKillChaos contains the prepration and injection steps for the experiment
func PrepareProcessKillChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	var pids []int

	for _, pid := range strings.Split(experimentsDetails.ProcessIds, ",") {

		p, err := strconv.Atoi(pid)
		if err != nil {
			return errors.Errorf("unable to convert process id %s to integer, err: %v", pid, err)
		}

		pids = append(pids, p)
	}

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err := injectChaosInSerialMode(experimentsDetails, pids, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	case "parallel":
		if err := injectChaosInParallelMode(experimentsDetails, pids, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
	}

	// wait for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	return nil
}

// injectChaosInSerialMode kills the processes in serial mode i.e. one after the other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, pids []int, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		log.Infof("[Info]: Target processes list: %v", pids)

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos in VM instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		// kill the processes
		for i, pid := range pids {

			timeDuration := 60 * time.Second

			log.Infof("[Chaos]: Killing %d process", pid)
			feedback, payload, err := messages.SendMessageToAgent(chaosDetails.WebsocketConnections[0], "EXECUTE_EXPERIMENT", []int{pid}, &timeDuration)
			if err != nil {
				return errors.Errorf("failed to send message to agent, err: %v", err)
			}

			common.SetTargets(strconv.Itoa(pid), "injected", "Process", chaosDetails)

			if err := messages.ValidateAgentFeedback(feedback, payload); err != nil {
				return errors.Errorf("error occured while killing %d process, err: %v", pid, err)
			}

			log.Infof("[Chaos]: %d process killed successfully", pid)

			// run the probes during chaos
			// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
			if len(resultDetails.ProbeDetails) != 0 && i == 0 {
				if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return err
				}
			}

			// wait for the chaos interval
			log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
			if err := common.WaitForDurationAndCheckLiveness(chaosDetails.WebsocketConnections, []string{experimentsDetails.AgentEndpoint}, experimentsDetails.ChaosInterval, nil, nil); err != nil {
				return errors.Errorf("error occured during liveness check, err: %v", err)
			}
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	return nil
}

// injectChaosInParallelMode kills the processes in parallel mode i.e. all at once
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, pids []int, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		log.Infof("[Info]: Target processes list: %v", pids)

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos in VM instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		timeDuration := 60 * time.Second

		// kill the processes
		log.Infof("[Chaos]: Killing %v processes", pids)
		feedback, payload, err := messages.SendMessageToAgent(chaosDetails.WebsocketConnections[0], "EXECUTE_EXPERIMENT", pids, &timeDuration)
		if err != nil {
			return errors.Errorf("failed to send message to agent, err: %v", err)
		}

		for _, pid := range pids {
			common.SetTargets(strconv.Itoa(pid), "injected", "Process", chaosDetails)
		}

		if err := messages.ValidateAgentFeedback(feedback, payload); err != nil {
			return errors.Errorf("error occured while trying to kill the process, err: %v", err)
		}

		log.Infof("[Chaos]: %v processes killed successfully", pids)

		// run the probes during chaos
		if len(resultDetails.ProbeDetails) != 0 {
			if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
				return err
			}
		}

		// wait for the chaos interval
		log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
		if err := common.WaitForDurationAndCheckLiveness(chaosDetails.WebsocketConnections, []string{experimentsDetails.AgentEndpoint}, experimentsDetails.ChaosInterval, nil, nil); err != nil {
			return errors.Errorf("error occured during liveness check, err: %v", err)
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	return nil
}
