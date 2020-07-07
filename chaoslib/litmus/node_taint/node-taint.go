package node_taint

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-taint/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//PrepareNodeTaint contains the prepration steps before chaos injection
func PrepareNodeTaint(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.AppNode + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// taint the application node
	err := TaintNode(experimentsDetails, clients)
	if err != nil {
		return err
	}

	// Verify the status of AUT after reschedule
	log.Info("[Status]: Verify the status of AUT after reschedule")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		return errors.Errorf("Application status check failed due to, err: %v", err)
	}

	// Verify the status of Auxiliary Applications after reschedule
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running")
		err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, clients)
		if err != nil {
			return errors.Errorf("Auxiliary Application status check failed due to %v", err)
		}
	}

	// Wait for Chaos Duration
	log.Infof("[Wait]: Waiting for the %vs chaos duration", strconv.Itoa(experimentsDetails.ChaosDuration))
	waitForChaosDuration(experimentsDetails)

	// remove taint from the application node
	err = RemoveTaintFromNode(experimentsDetails, clients)
	if err != nil {
		return err
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}
	return nil
}

//waitForRampTime waits for the given ramp time duration (in seconds)
func waitForRampTime(experimentsDetails *experimentTypes.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.RampTime) * time.Second)
}

//waitForChaosDuration waits for the given chaos duration (in seconds)
func waitForChaosDuration(experimentsDetails *experimentTypes.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.ChaosDuration) * time.Second)
}

// TaintNode taint the application node
func TaintNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	// get the taint labels & effect
	TaintKey, TaintValue, TaintEffect := GetTaintDetails(experimentsDetails)

	// get the node details
	node, err := clients.KubeClient.CoreV1().Nodes().Get(experimentsDetails.AppNode, v1.GetOptions{})
	if err != nil || node == nil {
		return errors.Errorf("failed to get %v node, due to err: %v", experimentsDetails.AppNode, err)
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
			return fmt.Errorf("failed to update %v node after adding taints, due to err: %v", experimentsDetails.AppNode, err)
		}
	}

	log.Infof("Successfully added taint on node %v", experimentsDetails.AppNode)
	return nil
}

// RemoveTaintFromNode remove the taint from the application node
func RemoveTaintFromNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	// Get the taint key
	TaintLabel := strings.Split(experimentsDetails.Taints, ":")
	TaintKey := strings.Split(TaintLabel[0], "=")[0]

	// get the node details
	node, err := clients.KubeClient.CoreV1().Nodes().Get(experimentsDetails.AppNode, v1.GetOptions{})
	if err != nil || node == nil {
		return errors.Errorf("failed to get %v node, due to err: %v", experimentsDetails.AppNode, err)
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
			return fmt.Errorf("failed to update %v node after removing taints, due to err: %v", experimentsDetails.AppNode, err)
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

	if len(TaintLabel) >= 2 {
		TaintValue = TaintLabel[1]
	}
	if len(Taints) >= 2 {
		TaintEffect = Taints[1]
	}

	return TaintKey, TaintValue, TaintEffect

}
