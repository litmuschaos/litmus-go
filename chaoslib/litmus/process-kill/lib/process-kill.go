package lib

import (
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/guest-os/process-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/messages"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

// PrepareProcessKillChaos contains the prepration and injection steps for the experiment
func PrepareProcessKillChaos(conn *websocket.Conn, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	processIdList := strings.Split(experimentsDetails.ProcessIds, ",")
	if len(processIdList) == 0 {
		return errors.Errorf("no process ID found")
	}

	var pids []int

	for _, pid := range processIdList {

		p, err := strconv.Atoi(pid)
		if err != nil {
			return errors.Errorf("unable to convert process id %s to integer, %v", pid, err)
		}

		pids = append(pids, p)
	}

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err := injectChaosInSerialMode(experimentsDetails, pids, conn, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	case "parallel":
		if err := injectChaosInParallelMode(experimentsDetails, pids, conn, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
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
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, pids []int, conn *websocket.Conn, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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

			log.Infof("[Chaos]: Killing %s process", strconv.Itoa(pid))
			if err := messages.SendMessageToAgent(conn, "EXECUTE_EXPERIMENT", experimentTypes.Processes{PIDs: []int{pid}}); err != nil {
				return errors.Errorf("failed to send message to agent, %v", err)
			}

			common.SetTargets(strconv.Itoa(pid), "injected", "Process", chaosDetails)

			feedback, payload, err := messages.ListenForAgentMessage(conn)
			if err != nil {
				return errors.Errorf("error during reception of message from agent, %v", err)
			}

			if feedback != "ACTION_SUCCESSFUL" {
				if feedback == "ERROR" {

					agentError, err := messages.GetErrorMessage(payload)
					if err != nil {
						return errors.Errorf("failed to interpret error message from agent, ", err)
					}

					return errors.Errorf("error occured while killing %v process, %s", pid, agentError)
				}

				return errors.Errorf("unintelligible feedback received from agent: %s", feedback)
			}

			log.Infof("[Chaos]: %s process killed successfully", strconv.Itoa(pid))

			// run the probes during chaos
			// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
			if len(resultDetails.ProbeDetails) != 0 && i == 0 {
				if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails, conn); err != nil {
					return err
				}
			}

			// wait for the chaos interval
			log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	return nil
}

// injectChaosInParallelMode kills the processes in parallel mode i.e. all at once
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, pids []int, conn *websocket.Conn, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
		log.Infof("[Chaos]: Killing %v processes", pids)
		if err := messages.SendMessageToAgent(conn, "EXECUTE_EXPERIMENT", experimentTypes.Processes{PIDs: pids}); err != nil {
			return errors.Errorf("failed to send message to agent, %v", err)
		}

		for _, pid := range pids {
			common.SetTargets(strconv.Itoa(pid), "injected", "Process", chaosDetails)
		}

		feedback, payload, err := messages.ListenForAgentMessage(conn)
		if err != nil {
			return errors.Errorf("error during reception of message from agent, %v", err)
		}

		if feedback != "ACTION_SUCCESSFUL" {
			if feedback == "ERROR" {

				agentError, err := messages.GetErrorMessage(payload)
				if err != nil {
					return errors.Errorf("failed to interpret error message from agent, %v", err)
				}

				return errors.Errorf("error during process kill, %s", agentError)
			}

			return errors.Errorf("unintelligible feedback received from agent: %s", feedback)
		}

		log.Infof("[Chaos]: %v processes killed successfully", pids)

		// run the probes during chaos
		if len(resultDetails.ProbeDetails) != 0 {
			if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails, conn); err != nil {
				return err
			}
		}

		// wait for chaos interval
		log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
		common.WaitForDuration(experimentsDetails.ChaosInterval)

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	return nil
}
