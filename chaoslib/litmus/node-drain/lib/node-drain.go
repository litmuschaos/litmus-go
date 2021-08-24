package lib

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-drain/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	err           error
	inject, abort chan os.Signal
)

//PrepareNodeDrain contains the prepration steps before chaos injection
func PrepareNodeDrain(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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

	if experimentsDetails.TargetNode == "" {
		//Select node for kubelet-service-kill
		experimentsDetails.TargetNode, err = common.GetNodeName(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.NodeLabel, clients)
		if err != nil {
			return err
		}
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.TargetNode + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// watching for the abort signal and revert the chaos
	go abortWatcher(experimentsDetails, clients, resultDetails, chaosDetails, eventsDetails)

	// Drain the application node
	if err := drainNode(experimentsDetails, clients, chaosDetails); err != nil {
		return err
	}

	// Verify the status of AUT after reschedule
	log.Info("[Status]: Verify the status of AUT after reschedule")
	if err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		return errors.Errorf("application status check failed, err: %v", err)
	}

	// Verify the status of Auxiliary Applications after reschedule
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running")
		if err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			return errors.Errorf("auxiliary Applications status check failed, err: %v", err)
		}
	}

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	common.WaitForDuration(experimentsDetails.ChaosDuration)

	log.Info("[Chaos]: Stopping the experiment")

	// Uncordon the application node
	if err := uncordonNode(experimentsDetails, clients, chaosDetails); err != nil {
		return err
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// drainNode drain the application node
func drainNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:
		log.Infof("[Inject]: Draining the %v node", experimentsDetails.TargetNode)

		command := exec.Command("kubectl", "drain", experimentsDetails.TargetNode, "--ignore-daemonsets", "--delete-local-data", "--force", "--timeout", strconv.Itoa(experimentsDetails.ChaosDuration)+"s")
		var out, stderr bytes.Buffer
		command.Stdout = &out
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			log.Infof("Error String: %v", stderr.String())
			return errors.Errorf("Unable to drain the %v node, err: %v", experimentsDetails.TargetNode, err)
		}

		common.SetTargets(experimentsDetails.TargetNode, "injected", "node", chaosDetails)

		return retry.
			Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
			Wait(time.Duration(experimentsDetails.Delay) * time.Second).
			Try(func(attempt uint) error {
				nodeSpec, err := clients.KubeClient.CoreV1().Nodes().Get(experimentsDetails.TargetNode, v1.GetOptions{})
				if err != nil {
					return err
				}
				if !nodeSpec.Spec.Unschedulable {
					return errors.Errorf("%v node is not in unschedulable state", experimentsDetails.TargetNode)
				}
				return nil
			})
	}
	return nil
}

// uncordonNode uncordon the application node
func uncordonNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	log.Infof("[Recover]: Uncordon the %v node", experimentsDetails.TargetNode)

	command := exec.Command("kubectl", "uncordon", experimentsDetails.TargetNode)
	var out, stderr bytes.Buffer
	command.Stdout = &out
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		log.Infof("Error String: %v", stderr.String())
		return errors.Errorf("unable to uncordon the %v node, err: %v", experimentsDetails.TargetNode, err)
	}

	common.SetTargets(experimentsDetails.TargetNode, "reverted", "node", chaosDetails)

	return retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			nodeSpec, err := clients.KubeClient.CoreV1().Nodes().Get(experimentsDetails.TargetNode, v1.GetOptions{})
			if err != nil {
				return err
			}
			if nodeSpec.Spec.Unschedulable {
				return errors.Errorf("%v node is in unschedulable state", experimentsDetails.TargetNode)
			}
			return nil
		})
}

// abortWatcher continuosly watch for the abort signals
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails) {
	// waiting till the abort signal recieved
	<-abort

	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		if err := uncordonNode(experimentsDetails, clients, chaosDetails); err != nil {
			log.Errorf("Unable to uncordon the node, err: %v", err)
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	log.Info("Chaos Revert Completed")
	os.Exit(0)
}
