package lib

import (
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	err           error
	inject, abort chan os.Signal
)

//PrepareNodeDrain contains the preparation steps before chaos injection
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
			return stacktrace.Propagate(err, "could not get node name")
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
		return stacktrace.Propagate(err, "could not drain node")
	}

	// Verify the status of AUT after reschedule
	log.Info("[Status]: Verify the status of AUT after reschedule")
	if err = status.AUTStatusCheck(clients, chaosDetails); err != nil {
		log.Info("[Revert]: Reverting chaos because application status check failed")
		if uncordonErr := uncordonNode(experimentsDetails, clients, chaosDetails); uncordonErr != nil {
			return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(uncordonErr).Error())}
		}
		return err
	}

	// Verify the status of Auxiliary Applications after reschedule
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running")
		if err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			log.Info("[Revert]: Reverting chaos because auxiliary application status check failed")
			if uncordonErr := uncordonNode(experimentsDetails, clients, chaosDetails); uncordonErr != nil {
				return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(uncordonErr).Error())}
			}
			return err
		}
	}

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	common.WaitForDuration(experimentsDetails.ChaosDuration)

	log.Info("[Chaos]: Stopping the experiment")

	// Uncordon the application node
	if err := uncordonNode(experimentsDetails, clients, chaosDetails); err != nil {
		return stacktrace.Propagate(err, "could not uncordon the target node")
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// drainNode drain the target node
func drainNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:
		log.Infof("[Inject]: Draining the %v node", experimentsDetails.TargetNode)

		command := exec.Command("kubectl", "drain", experimentsDetails.TargetNode, "--ignore-daemonsets", "--delete-emptydir-data", "--force", "--timeout", strconv.Itoa(experimentsDetails.ChaosDuration)+"s")
		if err := common.RunCLICommands(command, "", fmt.Sprintf("{node: %s}", experimentsDetails.TargetNode), "failed to drain the target node", cerrors.ErrorTypeChaosInject); err != nil {
			return err
		}

		common.SetTargets(experimentsDetails.TargetNode, "injected", "node", chaosDetails)

		return retry.
			Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
			Wait(time.Duration(experimentsDetails.Delay) * time.Second).
			Try(func(attempt uint) error {
				nodeSpec, err := clients.KubeClient.CoreV1().Nodes().Get(context.Background(), experimentsDetails.TargetNode, v1.GetOptions{})
				if err != nil {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{node: %s}", experimentsDetails.TargetNode), Reason: err.Error()}
				}
				if !nodeSpec.Spec.Unschedulable {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{node: %s}", experimentsDetails.TargetNode), Reason: "node is not in unschedule state"}
				}
				return nil
			})
	}
	return nil
}

// uncordonNode uncordon the application node
func uncordonNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	targetNodes := strings.Split(experimentsDetails.TargetNode, ",")
	for _, targetNode := range targetNodes {

		//Check node exist before uncordon the node
		_, err := clients.KubeClient.CoreV1().Nodes().Get(context.Background(), targetNode, v1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Infof("[Info]: The %v node is no longer exist, skip uncordon the node", targetNode)
				common.SetTargets(targetNode, "noLongerExist", "node", chaosDetails)
				continue
			} else {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{node: %s}", targetNode), Reason: err.Error()}
			}
		}

		log.Infof("[Recover]: Uncordon the %v node", targetNode)
		command := exec.Command("kubectl", "uncordon", targetNode)
		if err := common.RunCLICommands(command, "", fmt.Sprintf("{node: %s}", targetNode), "failed to uncordon the target node", cerrors.ErrorTypeChaosInject); err != nil {
			return err
		}
		common.SetTargets(targetNode, "reverted", "node", chaosDetails)
	}

	return retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			targetNodes := strings.Split(experimentsDetails.TargetNode, ",")
			for _, targetNode := range targetNodes {
				nodeSpec, err := clients.KubeClient.CoreV1().Nodes().Get(context.Background(), targetNode, v1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						continue
					} else {
						return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{node: %s}", targetNode), Reason: err.Error()}
					}
				}
				if nodeSpec.Spec.Unschedulable {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{node: %s}", targetNode), Reason: "target node is in unschedule state"}
				}
			}
			return nil
		})
}

// abortWatcher continuously watch for the abort signals
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails) {
	// waiting till the abort signal received
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
