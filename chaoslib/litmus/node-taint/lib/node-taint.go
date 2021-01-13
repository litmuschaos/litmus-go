package lib

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-taint/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//PrepareNodeTaint contains the prepration steps before chaos injection
func PrepareNodeTaint(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.TargetNode == "" {
		//Select node for kubelet-service-kill
		experimentsDetails.TargetNode, err = common.GetNodeName(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
		if err != nil {
			return err
		}
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.TargetNode + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// taint the application node
	err := TaintNode(experimentsDetails, clients)
	if err != nil {
		return err
	}

	// Verify the status of AUT after reschedule
	log.Info("[Status]: Verify the status of AUT after reschedule")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Application status check failed, err: %v", err)
	}

	// Verify the status of Auxiliary Applications after reschedule
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running")
		err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			return errors.Errorf("Auxiliary Applications status check failed, err: %v", err)
		}
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)

loop:
	for {
		endTime = time.After(timeDelay)
		select {
		case <-signChan:
			log.Info("[Chaos]: Killing process started because of terminated signal received")
			// updating the chaosresult after stopped
			failStep := "Node Taint injection stopped!"
			types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
			result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

			// generating summary event in chaosengine
			msg := experimentsDetails.ExperimentName + " experiment has been aborted"
			types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

			// generating summary event in chaosresult
			types.SetResultEventAttributes(eventsDetails, types.StoppedVerdict, msg, "Warning", resultDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")

			if err = RemoveTaintFromNode(experimentsDetails, clients); err != nil {
				log.Errorf("unable to remove taint from the node, err :%v", err)

			}
			os.Exit(1)
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			endTime = nil
			break loop
		}
	}

	// remove taint from the application node
	err = RemoveTaintFromNode(experimentsDetails, clients)
	if err != nil {
		return err
	}

	// Checking the status of target nodes
	log.Info("[Status]: Getting the status of target nodes")
	err = status.CheckNodeStatus(experimentsDetails.TargetNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		log.Warnf("Target nodes are not in the ready state, you may need to manually recover the node, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// TaintNode taint the application node
func TaintNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	// get the taint labels & effect
	TaintKey, TaintValue, TaintEffect := GetTaintDetails(experimentsDetails)

	log.Infof("Add %v taints to the %v node", TaintKey+"="+TaintValue+":"+TaintEffect, experimentsDetails.TargetNode)

	// get the node details
	node, err := clients.KubeClient.CoreV1().Nodes().Get(experimentsDetails.TargetNode, v1.GetOptions{})
	if err != nil || node == nil {
		return errors.Errorf("failed to get %v node, err: %v", experimentsDetails.TargetNode, err)
	}

	// check if the taint already exists
	tainted := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == TaintKey {
			tainted = true
			break
		}
	}

	if !tainted {
		node.Spec.Taints = append(node.Spec.Taints, apiv1.Taint{
			Key:    TaintKey,
			Value:  TaintValue,
			Effect: apiv1.TaintEffect(TaintEffect),
		})

		updatedNodeWithTaint, err := clients.KubeClient.CoreV1().Nodes().Update(node)
		if err != nil || updatedNodeWithTaint == nil {
			return fmt.Errorf("failed to update %v node after adding taints, err: %v", experimentsDetails.TargetNode, err)
		}
	}

	log.Infof("Successfully added taint in %v node", experimentsDetails.TargetNode)
	return nil
}

// RemoveTaintFromNode remove the taint from the application node
func RemoveTaintFromNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	// Get the taint key
	TaintLabel := strings.Split(experimentsDetails.Taints, ":")
	TaintKey := strings.Split(TaintLabel[0], "=")[0]

	// get the node details
	node, err := clients.KubeClient.CoreV1().Nodes().Get(experimentsDetails.TargetNode, v1.GetOptions{})
	if err != nil || node == nil {
		return errors.Errorf("failed to get %v node, err: %v", experimentsDetails.TargetNode, err)
	}

	// check if the taint already exists
	tainted := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == TaintKey {
			tainted = true
			break
		}
	}

	if tainted {
		var Newtaints []apiv1.Taint
		// remove all the taints with matching key
		for _, taint := range node.Spec.Taints {
			if taint.Key != TaintKey {
				Newtaints = append(Newtaints, taint)
			}
		}
		node.Spec.Taints = Newtaints
		updatedNodeWithTaint, err := clients.KubeClient.CoreV1().Nodes().Update(node)
		if err != nil || updatedNodeWithTaint == nil {
			return fmt.Errorf("failed to update %v node after removing taints, err: %v", experimentsDetails.TargetNode, err)
		}
	}

	log.Infof("Successfully removed taint from the %v node", node.Name)
	return nil
}

// GetTaintDetails return the key, value and effect for the taint
func GetTaintDetails(experimentsDetails *experimentTypes.ExperimentDetails) (string, string, string) {
	TaintValue := "node-taint"
	TaintEffect := string(apiv1.TaintEffectNoExecute)

	Taints := strings.Split(experimentsDetails.Taints, ":")
	TaintLabel := strings.Split(Taints[0], "=")
	TaintKey := TaintLabel[0]

	// It will set the value for taint label from `TAINT` env, if provided
	// otherwise it will use the `node-taint` value as default value.
	if len(TaintLabel) >= 2 {
		TaintValue = TaintLabel[1]
	}
	// It will set the value for taint effect from `TAINT` env, if provided
	// otherwise it will use `NoExecute` value as default value.
	if len(Taints) >= 2 {
		TaintEffect = Taints[1]
	}

	return TaintKey, TaintValue, TaintEffect

}
