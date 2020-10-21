package lib

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-drain/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//PrepareNodeDrain contains the prepration steps before chaos injection
func PrepareNodeDrain(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Select node for node-drain
	targetNodeList, err := common.GetNodeList(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.TargetNode, experimentsDetails.NodesAffectedPerc, clients)
	if err != nil {
		return err
	}

	if experimentsDetails.Sequence == "serial" {
		if err = InjectChaosInSerialMode(experimentsDetails, targetNodeList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	} else {
		if err = InjectChaosInParallelMode(experimentsDetails, targetNodeList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// InjectChaosInSerialMode drain all the target nodes serially (one by one)
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetNodeList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	for _, appNode := range targetNodeList {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNode + " node"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		log.InfoWithValues("[Info]: Details of Node under chaos injection", logrus.Fields{
			"NodeName": appNode,
		})

		// Drain the application node
		// Drain the application node
		if err := DrainNode(appNode, clients); err != nil {
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
				failStep := "Node Drain injection stopped!"
				types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
				result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

				// generating summary event in chaosengine
				msg := experimentsDetails.ExperimentName + " experiment has been aborted"
				types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

				// generating summary event in chaosresult
				types.SetResultEventAttributes(eventsDetails, types.StoppedVerdict, msg, "Warning", resultDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")

				if err = UncordonNodeInSerial(appNode, clients); err != nil {
					log.Errorf("unable to uncordon node, err: %v", err)

				}
				os.Exit(1)
			case <-endTime:
				log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
				endTime = nil
				break loop
			}
		}

		// Uncordon the application node
		if err = UncordonNodeInSerial(appNode, clients); err != nil {
			return err
		}
	}
	return nil
}

// InjectChaosInParallelMode drain all the target nodes in parallel mode (all at once)
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetNodeList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	for _, appNode := range targetNodeList {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNode + " node"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		log.InfoWithValues("[Info]: Details of Node under chaos injection", logrus.Fields{
			"NodeName": appNode,
		})

		// Drain the application node
		if err := DrainNode(appNode, clients); err != nil {
			return err
		}
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
			failStep := "Node Drain injection stopped!"
			types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
			result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

			// generating summary event in chaosengine
			msg := experimentsDetails.ExperimentName + " experiment has been aborted"
			types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

			// generating summary event in chaosresult
			types.SetResultEventAttributes(eventsDetails, types.StoppedVerdict, msg, "Warning", resultDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")

			if err = UncordonNodeInParallel(targetNodeList, clients); err != nil {
				log.Errorf("unable to uncordon nodes, err: %v", err)

			}
			os.Exit(1)
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			endTime = nil
			break loop
		}
	}

	// Uncordon the application node
	if err = UncordonNodeInParallel(targetNodeList, clients); err != nil {
		return err
	}

	return nil
}

// DrainNode drain the application node
func DrainNode(appNode string, clients clients.ClientSets) error {

	log.Infof("[Inject]: Draining the %v node", appNode)

	command := exec.Command("kubectl", "drain", appNode, "--ignore-daemonsets", "--delete-local-data", "--force")
	var out, stderr bytes.Buffer
	command.Stdout = &out
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		log.Infof("Error String: %v", stderr.String())
		return fmt.Errorf("Unable to drain the %v node, err: %v", appNode, err)
	}

	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			nodeSpec, err := clients.KubeClient.CoreV1().Nodes().Get(appNode, v1.GetOptions{})
			if err != nil {
				return err
			}
			if !nodeSpec.Spec.Unschedulable {
				return errors.Errorf("%v node is not in unschedulable state", appNode)
			}
			return nil
		})

	return err
}

// UncordonNodeInSerial uncordon the application node
// Triggered by either timeout of chaos duration or termination of the experiment
func UncordonNodeInSerial(appNode string, clients clients.ClientSets) error {

	log.Infof("[Recover]: Uncordon the %v node", appNode)

	command := exec.Command("kubectl", "uncordon", appNode)
	var out, stderr bytes.Buffer
	command.Stdout = &out
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		log.Infof("Error String: %v", stderr.String())
		return fmt.Errorf("Unable to uncordon the %v node, err: %v", appNode, err)
	}

	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			nodeSpec, err := clients.KubeClient.CoreV1().Nodes().Get(appNode, v1.GetOptions{})
			if err != nil {
				return err
			}
			if nodeSpec.Spec.Unschedulable {
				return errors.Errorf("%v node is in unschedulable state", appNode)
			}
			return nil
		})

	return nil
}

// UncordonNodeInParallel function to uncorden all the nodes
// Triggered by either timeout of chaos duration or termination of the experiment
func UncordonNodeInParallel(targetNodeList []string, clients clients.ClientSets) error {

	for _, node := range targetNodeList {

		if err := UncordonNodeInSerial(node, clients); err != nil {
			return err
		}
	}
	return nil
}
