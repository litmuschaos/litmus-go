package lib

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-drain/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//PrepareNodeDrain contains the prepration steps before chaos injection
func PrepareNodeDrain(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.AppNode + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// Drain the application node
	err := DrainNode(experimentsDetails, clients)
	if err != nil {
		return err
	}

	// Verify the status of AUT after reschedule
	log.Info("[Status]: Verify the status of AUT after reschedule")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Application status check failed due to, err: %v", err)
	}

	// Verify the status of Auxiliary Applications after reschedule
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running")
		err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			return errors.Errorf("Auxiliary Application status check failed due to %v", err)
		}
	}

	// Wait for Chaos Duration
	log.Infof("[Wait]: Waiting for the %vs chaos duration", strconv.Itoa(experimentsDetails.ChaosDuration))
	common.WaitForDuration(experimentsDetails.ChaosDuration)

	// Uncordon the application node
	err = UncordonNode(experimentsDetails, clients)
	if err != nil {
		return err
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// DrainNode drain the application node
func DrainNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	log.Infof("[Inject]: Draining the %v node", experimentsDetails.AppNode)

	command := exec.Command("kubectl", "drain", experimentsDetails.AppNode, "--ignore-daemonsets", "--delete-local-data", "--force")
	var out, stderr bytes.Buffer
	command.Stdout = &out
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		log.Infof("Error String: %v", stderr.String())
		return fmt.Errorf("Unable to drain the %v node, err: %v", experimentsDetails.AppNode, err)
	}

	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			nodeSpec, err := clients.KubeClient.CoreV1().Nodes().Get(experimentsDetails.AppNode, v1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to get the %v node, err: %v", experimentsDetails.AppNode, err)
			}
			if !nodeSpec.Spec.Unschedulable {
				return errors.Errorf("Unable to drain %v node", experimentsDetails.AppNode)
			}
			return nil
		})

	return nil
}

// UncordonNode uncordon the application node
func UncordonNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	log.Infof("[Recover]: Uncordon the %v node", experimentsDetails.AppNode)

	command := exec.Command("kubectl", "uncordon", experimentsDetails.AppNode)
	var out, stderr bytes.Buffer
	command.Stdout = &out
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		log.Infof("Error String: %v", stderr.String())
		return fmt.Errorf("Unable to uncordon the %v node, err: %v", experimentsDetails.AppNode, err)
	}

	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			nodeSpec, err := clients.KubeClient.CoreV1().Nodes().Get(experimentsDetails.AppNode, v1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to get the %v node, err: %v", experimentsDetails.AppNode, err)
			}
			if nodeSpec.Spec.Unschedulable {
				return errors.Errorf("Unable to uncordon %v node", experimentsDetails.AppNode)
			}
			return nil
		})

	return nil
}
