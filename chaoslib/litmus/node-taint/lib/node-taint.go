package lib

import (
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-taint/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	err           error
	inject, abort chan os.Signal
)

//PrepareNodeTaint contains the preparation steps before chaos injection
func PrepareNodeTaint(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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

	// taint the application node
	if err := taintNode(experimentsDetails, clients, chaosDetails); err != nil {
		return stacktrace.Propagate(err, "could not taint node")
	}

	// Verify the status of AUT after reschedule
	log.Info("[Status]: Verify the status of AUT after reschedule")
	if err = status.AUTStatusCheck(clients, chaosDetails); err != nil {
		log.Info("[Revert]: Reverting chaos because application status check failed")
		if taintErr := removeTaintFromNode(experimentsDetails, clients, chaosDetails); taintErr != nil {
			return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(taintErr).Error())}
		}
		return err
	}

	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running")
		if err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			log.Info("[Revert]: Reverting chaos because auxiliary application status check failed")
			if taintErr := removeTaintFromNode(experimentsDetails, clients, chaosDetails); taintErr != nil {
				return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(taintErr).Error())}
			}
			return err
		}
	}

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	common.WaitForDuration(experimentsDetails.ChaosDuration)

	log.Info("[Chaos]: Stopping the experiment")

	// remove taint from the application node
	if err := removeTaintFromNode(experimentsDetails, clients, chaosDetails); err != nil {
		return stacktrace.Propagate(err, "could not remove taint from node")
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// taintNode taint the application node
func taintNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	// get the taint labels & effect
	taintKey, taintValue, taintEffect := getTaintDetails(experimentsDetails)

	log.Infof("Add %v taints to the %v node", taintKey+"="+taintValue+":"+taintEffect, experimentsDetails.TargetNode)

	// get the node details
	node, err := clients.KubeClient.CoreV1().Nodes().Get(context.Background(), experimentsDetails.TargetNode, v1.GetOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{nodeName: %s}", experimentsDetails.TargetNode), Reason: err.Error()}
	}

	// check if the taint already exists
	tainted := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == taintKey {
			tainted = true
			break
		}
	}

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:
		if !tainted {
			node.Spec.Taints = append(node.Spec.Taints, apiv1.Taint{
				Key:    taintKey,
				Value:  taintValue,
				Effect: apiv1.TaintEffect(taintEffect),
			})

			_, err := clients.KubeClient.CoreV1().Nodes().Update(context.Background(), node, v1.UpdateOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{nodeName: %s}", node.Name), Reason: fmt.Sprintf("failed to add taints: %s", err.Error())}
			}
		}

		common.SetTargets(node.Name, "injected", "node", chaosDetails)

		log.Infof("Successfully added taint in %v node", experimentsDetails.TargetNode)
	}
	return nil
}

// removeTaintFromNode remove the taint from the application node
func removeTaintFromNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	// Get the taint key
	taintLabel := strings.Split(experimentsDetails.Taints, ":")
	taintKey := strings.Split(taintLabel[0], "=")[0]

	// get the node details
	node, err := clients.KubeClient.CoreV1().Nodes().Get(context.Background(), experimentsDetails.TargetNode, v1.GetOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{nodeName: %s}", experimentsDetails.TargetNode), Reason: err.Error()}
	}

	// check if the taint already exists
	tainted := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == taintKey {
			tainted = true
			break
		}
	}

	if tainted {
		var newTaints []apiv1.Taint
		// remove all the taints with matching key
		for _, taint := range node.Spec.Taints {
			if taint.Key != taintKey {
				newTaints = append(newTaints, taint)
			}
		}
		node.Spec.Taints = newTaints
		updatedNodeWithTaint, err := clients.KubeClient.CoreV1().Nodes().Update(context.Background(), node, v1.UpdateOptions{})
		if err != nil || updatedNodeWithTaint == nil {
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{nodeName: %s}", node.Name), Reason: fmt.Sprintf("failed to remove taints: %s", err.Error())}
		}
	}

	common.SetTargets(node.Name, "reverted", "node", chaosDetails)

	log.Infof("Successfully removed taint from the %v node", node.Name)
	return nil
}

// GetTaintDetails return the key, value and effect for the taint
func getTaintDetails(experimentsDetails *experimentTypes.ExperimentDetails) (string, string, string) {
	taintValue := "node-taint"
	taintEffect := string(apiv1.TaintEffectNoExecute)

	taints := strings.Split(experimentsDetails.Taints, ":")
	taintLabel := strings.Split(taints[0], "=")
	taintKey := taintLabel[0]

	// It will set the value for taint label from `TAINT` env, if provided
	// otherwise it will use the `node-taint` value as default value.
	if len(taintLabel) >= 2 {
		taintValue = taintLabel[1]
	}
	// It will set the value for taint effect from `TAINT` env, if provided
	// otherwise it will use `NoExecute` value as default value.
	if len(taints) >= 2 {
		taintEffect = taints[1]
	}

	return taintKey, taintValue, taintEffect
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
		if err := removeTaintFromNode(experimentsDetails, clients, chaosDetails); err != nil {
			log.Errorf("Unable to untaint node, err: %v", err)
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	log.Info("Chaos Revert Completed")
	os.Exit(0)
}
