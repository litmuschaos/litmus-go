package lib

import (
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/machine/common/messages"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/os/cpu-stress/types"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

var inject, abort chan os.Signal
var timeDuration = 60 * time.Second
var chaosRevert sync.WaitGroup
var underChaosEndpoints []int

type cpuStressParams struct {
	Workers string
	Load    string
	Timeout string
}

// InjectCPUStressChaos contains the prepration and injection steps for the experiment
func InjectCPUStressChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	// waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	agentEndpointList := strings.Split(experimentsDetails.AgentEndpoints, ",")

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:

		// watching for the abort signal and revert the chaos
		go AbortWatcher(chaosDetails.WebsocketConnections, agentEndpointList, abort, chaosDetails)
		chaosRevert.Add(1)

		switch strings.ToLower(experimentsDetails.Sequence) {
		case "serial":
			if err := injectChaosInSerialMode(experimentsDetails, agentEndpointList, clients, resultDetails, eventsDetails, chaosDetails, abort); err != nil {
				return err
			}
		case "parallel":
			if err := injectChaosInParallelMode(experimentsDetails, agentEndpointList, clients, resultDetails, eventsDetails, chaosDetails, abort); err != nil {
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
	}

	return nil
}

// injectChaosInSerialMode injects CPU stress chaos in serial mode i.e. one after the other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, agentEndpointList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, abort chan os.Signal) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos in VM instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		for i := range agentEndpointList {

			log.Infof("[Chaos]: Injecting CPU stress for %s agent endpoint", agentEndpointList[i])
			feedback, payload, err := messages.SendMessageToAgent(chaosDetails.WebsocketConnections[i], "EXECUTE_EXPERIMENT", cpuStressParams{experimentsDetails.CPUs, experimentsDetails.LoadPercentage, strconv.Itoa(experimentsDetails.ChaosInterval)}, &timeDuration)
			if err != nil {
				return errors.Errorf("failed while sending message to agent, err: %v", err)
			}

			// ACTION_SUCCESSFUL feedback is received only if the cpu stress chaos has been injected successfully
			if feedback != "ACTION_SUCCESSFUL" {
				if feedback == "ERROR" {

					agentError, err := messages.GetErrorMessage(payload)
					if err != nil {
						return errors.Errorf("failed to interpret error message from agent, err: %v", err)
					}

					return errors.Errorf("error occured while injecting CPU stress chaos for %s agent endpoint, err: %s", agentEndpointList[i], agentError)
				}

				return errors.Errorf("unintelligible feedback received from agent: %s", feedback)
			}

			underChaosEndpoints = append(underChaosEndpoints, i)

			common.SetTargets(agentEndpointList[i], "injected", "Machine", chaosDetails)

			log.Infof("[Chaos]: CPU stress chaos injected successfully in %s agent endpoint", agentEndpointList[i])

			// run the probes during chaos
			// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
			if len(resultDetails.ProbeDetails) != 0 && i == 0 {
				if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return err
				}
			}

			// wait for the chaos interval
			log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
			if err := common.WaitForDurationAndCheckLiveness([]*websocket.Conn{chaosDetails.WebsocketConnections[i]}, []string{agentEndpointList[i]}, experimentsDetails.ChaosInterval, abort, &chaosRevert); err != nil {
				return errors.Errorf("error occured during liveness check, err: %v", err)
			}

			log.Infof("[Chaos]: Reverting CPU stress for %s agent endpoint", agentEndpointList[i])
			feedback, payload, err = messages.SendMessageToAgent(chaosDetails.WebsocketConnections[i], "REVERT_CHAOS", nil, &timeDuration)
			if err != nil {
				return errors.Errorf("failed while sending message to agent, err: %v", err)
			}

			// ACTION_SUCCESSFUL feedback is received only if the cpu stress chaos has been injected successfully
			if feedback != "ACTION_SUCCESSFUL" {
				if feedback == "ERROR" {

					agentError, err := messages.GetErrorMessage(payload)
					if err != nil {
						return errors.Errorf("failed to interpret error message from agent, err: %v", err)
					}

					return errors.Errorf("error occured while reverting CPU stress chaos for %s agent endpoint, err: %s", agentEndpointList[i], agentError)
				}

				return errors.Errorf("unintelligible feedback received from agent: %s", feedback)
			}

			underChaosEndpoints = underChaosEndpoints[:len(underChaosEndpoints)-1]

			common.SetTargets(agentEndpointList[i], "reverted", "Machine", chaosDetails)
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	return nil
}

// injectChaosInParallelMode injects CPU stress chaos in parallel mode i.e. all at once
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, agentEndpointList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, abort chan os.Signal) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos in VM instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		// inject cpu stress chaos
		for i := range agentEndpointList {

			log.Infof("[Chaos]: Injecting CPU stress for %s agent endpoint", agentEndpointList[i])
			feedback, payload, err := messages.SendMessageToAgent(chaosDetails.WebsocketConnections[i], "EXECUTE_EXPERIMENT", cpuStressParams{experimentsDetails.CPUs, experimentsDetails.LoadPercentage, strconv.Itoa(experimentsDetails.ChaosInterval)}, &timeDuration)
			if err != nil {
				return errors.Errorf("failed while sending message to agent, err: %v", err)
			}

			// ACTION_SUCCESSFUL feedback is received only if the cpu stress chaos has been injected successfully
			if feedback != "ACTION_SUCCESSFUL" {
				if feedback == "ERROR" {

					agentError, err := messages.GetErrorMessage(payload)
					if err != nil {
						return errors.Errorf("failed to interpret error message from agent, err: %v", err)
					}

					return errors.Errorf("error occured while injecting CPU stress chaos for %s agent endpoint, err: %s", agentEndpointList[i], agentError)
				}

				return errors.Errorf("unintelligible feedback received from agent: %s", feedback)
			}

			underChaosEndpoints = append(underChaosEndpoints, i)

			common.SetTargets(agentEndpointList[i], "injected", "Machine", chaosDetails)

			log.Infof("[Chaos]: CPU stress chaos injected successfully in %s agent endpoint", agentEndpointList[i])
		}

		// run the probes during chaos
		// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
		if len(resultDetails.ProbeDetails) != 0 {
			if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
				return err
			}
		}

		// wait for the chaos interval
		log.Infof("[Wait]: Waiting for chaos interval of %vs", experimentsDetails.ChaosInterval)
		if err := common.WaitForDurationAndCheckLiveness(chaosDetails.WebsocketConnections, agentEndpointList, experimentsDetails.ChaosInterval, abort, &chaosRevert); err != nil {
			return errors.Errorf("error occured during liveness check, err: %v", err)
		}

		for i := range agentEndpointList {

			log.Infof("[Chaos]: Reverting CPU stress for %s agent endpoint", agentEndpointList[i])
			feedback, payload, err := messages.SendMessageToAgent(chaosDetails.WebsocketConnections[i], "REVERT_CHAOS", nil, &timeDuration)
			if err != nil {
				return errors.Errorf("failed while sending message to agent, err: %v", err)
			}

			// ACTION_SUCCESSFUL feedback is received only if the cpu stress chaos has been injected successfully
			if feedback != "ACTION_SUCCESSFUL" {
				if feedback == "ERROR" {

					agentError, err := messages.GetErrorMessage(payload)
					if err != nil {
						return errors.Errorf("failed to interpret error message from agent, err: %v", err)
					}

					return errors.Errorf("error occured while reverting CPU stress chaos for %s agent endpoint, err: %s", agentEndpointList[i], agentError)
				}

				return errors.Errorf("unintelligible feedback received from agent: %s", feedback)
			}

			common.SetTargets(agentEndpointList[i], "reverted", "Machine", chaosDetails)

			underChaosEndpoints = underChaosEndpoints[1:]
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}

	return nil
}

// AbortWatcher will watch for the abort signal and revert the chaos
func AbortWatcher(connections []*websocket.Conn, agentEndpointList []string, abort chan os.Signal, chaosDetails *types.ChaosDetails) {

	<-abort

	log.Info("[Abort]: Chaos Revert Started")

	for _, i := range underChaosEndpoints {

		log.Infof("[Abort]: Reverting CPU stress for %s agent endpoint", agentEndpointList[i])
		feedback, payload, err := messages.SendMessageToAgent(connections[i], "ABORT_EXPERIMENT", nil, &timeDuration)
		if err != nil {
			log.Errorf("unable to send abort chaos message to %s agent endpoint, err: %v", agentEndpointList[i], err)
		}

		// ACTION_SUCCESSFUL feedback is received only if the cpu stress chaos has been aborted successfully
		if feedback != "ACTION_SUCCESSFUL" {
			if feedback == "ERROR" {

				agentError, err := messages.GetErrorMessage(payload)
				if err != nil {
					log.Errorf("failed to interpret error message from agent, err: %v", err)
				}

				log.Errorf("error occured while aborting the experiment for %s agent endpoint, err: %s", agentEndpointList[i], agentError)
			}

			log.Errorf("unintelligible feedback received from agent: %s", feedback)
		}

		common.SetTargets(agentEndpointList[i], "reverted", "Machine", chaosDetails)
	}

	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
