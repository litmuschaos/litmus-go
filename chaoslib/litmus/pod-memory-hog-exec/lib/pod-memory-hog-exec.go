package lib

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-memory-hog-exec/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

var inject chan os.Signal

// stressMemory Uses the REST API to exec into the target container of the target pod
// The function will be constantly increasing the Memory utilisation until it reaches the maximum available or allowed number.
// Using the TOTAL_CHAOS_DURATION we will need to specify for how long this experiment will last
func stressMemory(MemoryConsumption, containerName, podName, namespace string, clients clients.ClientSets, stressErr chan error) {

	log.Infof("The memory consumption is: %v", MemoryConsumption)

	// It will contain all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}

	ddCmd := fmt.Sprintf("dd if=/dev/zero of=/dev/null bs=" + MemoryConsumption + "M")
	command := []string{"/bin/sh", "-c", ddCmd}

	litmusexec.SetExecCommandAttributes(&execCommandDetails, podName, containerName, namespace)
	_, err := litmusexec.Exec(&execCommandDetails, clients, command)

	stressErr <- err
}

//experimentMemory function orchestrates the experiment by calling the StressMemory function, of every container, of every pod that is targeted
func experimentMemory(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail.Label == "" {
		return errors.Errorf("please provide one of the appLabel or TARGET_PODS")
	}
	targetPodList, err := common.GetPodList(experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
	if err != nil {
		return err
	}

	podNames := []string{}
	for _, pod := range targetPodList.Items {
		podNames = append(podNames, pod.Name)
	}
	log.Infof("Target pods list for chaos, %v", podNames)

	//Get the target container name of the application pod
	if experimentsDetails.TargetContainer == "" {
		experimentsDetails.TargetContainer, err = common.GetTargetContainer(experimentsDetails.AppNS, targetPodList.Items[0].Name, clients)
		if err != nil {
			return errors.Errorf("unable to get the target container name, err: %v", err)
		}
	}

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(experimentsDetails, targetPodList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, targetPodList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
	}

	return nil
}

// injectChaosInSerialMode stressed the memory of all target application serially (one by one)
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList corev1.PodList, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM)

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		for _, pod := range targetPodList.Items {

			// creating err channel to recieve the error from the go routine
			stressErr := make(chan error)

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			log.InfoWithValues("[Chaos]: The Target application details", logrus.Fields{
				"Target Container":       experimentsDetails.TargetContainer,
				"Target Pod":             pod.Name,
				"Memory Consumption(MB)": experimentsDetails.MemoryConsumption,
			})
			go stressMemory(strconv.Itoa(experimentsDetails.MemoryConsumption), experimentsDetails.TargetContainer, pod.Name, experimentsDetails.AppNS, clients, stressErr)

			common.SetTargets(pod.Name, "injected", "pod", chaosDetails)

			log.Infof("[Chaos]:Waiting for: %vs", experimentsDetails.ChaosDuration)

		loop:
			for {
				endTime = time.After(timeDelay)
				select {
				case err := <-stressErr:
					// skipping the execution, if recieved any error other than 137, while executing stress command and marked result as fail
					// it will ignore the error code 137(oom kill), it will skip further execution and marked the result as pass
					// oom kill occurs if memory to be stressed exceed than the resource limit for the target container
					if err != nil {
						if strings.Contains(err.Error(), "137") {
							log.Warn("Chaos process OOM killed")
							return nil
						}
						return err
					}
				case <-signChan:
					log.Info("[Chaos]: Revert Started")
					if err := killStressMemorySerial(experimentsDetails.TargetContainer, pod.Name, experimentsDetails.AppNS, experimentsDetails.ChaosKillCmd, clients, chaosDetails); err != nil {
						log.Errorf("Error in Kill stress after abortion, err: %v", err)
					}
					// updating the chaosresult after stopped
					failStep := "Chaos injection stopped!"
					types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
					result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")
					log.Info("[Chaos]: Revert Completed")
					os.Exit(1)
				case <-endTime:
					log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
					endTime = nil
					break loop
				}
			}
			if err := killStressMemorySerial(experimentsDetails.TargetContainer, pod.Name, experimentsDetails.AppNS, experimentsDetails.ChaosKillCmd, clients, chaosDetails); err != nil {
				return err
			}
		}
	}
	return nil
}

// injectChaosInParallelMode stressed the memory of all target application in parallel mode (all at once)
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList corev1.PodList, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	// creating err channel to recieve the error from the go routine
	stressErr := make(chan error)

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM)

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		for _, pod := range targetPodList.Items {

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			log.InfoWithValues("[Chaos]: The Target application details", logrus.Fields{
				"Target Container":       experimentsDetails.TargetContainer,
				"Target Pod":             pod.Name,
				"Memory Consumption(MB)": experimentsDetails.MemoryConsumption,
			})

			go stressMemory(strconv.Itoa(experimentsDetails.MemoryConsumption), experimentsDetails.TargetContainer, pod.Name, experimentsDetails.AppNS, clients, stressErr)
		}
	}

	log.Infof("[Chaos]:Waiting for: %vs", experimentsDetails.ChaosDuration)

loop:
	for {
		endTime = time.After(timeDelay)
		select {
		case err := <-stressErr:
			// skipping the execution, if recieved any error other than 137, while executing stress command and marked result as fail
			// it will ignore the error code 137(oom kill), it will skip further execution and marked the result as pass
			// oom kill occurs if memory to be stressed exceed than the resource limit for the target container
			if err != nil {
				if strings.Contains(err.Error(), "137") {
					log.Warn("Chaos process OOM killed")
					return nil
				}
				return err
			}
		case <-signChan:
			log.Info("[Chaos]: Revert Started")
			if err := killStressMemoryParallel(experimentsDetails.TargetContainer, targetPodList, experimentsDetails.AppNS, experimentsDetails.ChaosKillCmd, clients, chaosDetails); err != nil {
				log.Errorf("Error in Kill stress after abortion, err: %v", err)
			}
			log.Info("[Chaos]: Revert Completed")
			os.Exit(1)
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			break loop
		}
	}
	if err := killStressMemoryParallel(experimentsDetails.TargetContainer, targetPodList, experimentsDetails.AppNS, experimentsDetails.ChaosKillCmd, clients, chaosDetails); err != nil {
		return err
	}

	return nil
}

//PrepareMemoryExecStress contains the chaos prepration and injection steps
func PrepareMemoryExecStress(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	//Starting the Memory stress experiment
	if err := experimentMemory(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
		return err
	}
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// killStressMemorySerial function to kill a stress process running inside target container
//  Triggered by either timeout of chaos duration or termination of the experiment
func killStressMemorySerial(containerName, podName, namespace, memFreeCmd string, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {
	// It will contains all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}

	command := []string{"/bin/sh", "-c", memFreeCmd}

	litmusexec.SetExecCommandAttributes(&execCommandDetails, podName, containerName, namespace)
	_, err := litmusexec.Exec(&execCommandDetails, clients, command)
	if err != nil {
		return errors.Errorf("Unable to kill stress process inside target container, err: %v", err)
	}
	common.SetTargets(podName, "reverted", "pod", chaosDetails)
	return nil
}

// killStressMemoryParallel function to kill all the stress process running inside target container
// Triggered by either timeout of chaos duration or termination of the experiment
func killStressMemoryParallel(containerName string, targetPodList corev1.PodList, namespace, memFreeCmd string, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	for _, pod := range targetPodList.Items {

		if err := killStressMemorySerial(containerName, pod.Name, namespace, memFreeCmd, clients, chaosDetails); err != nil {
			return err
		}
	}
	return nil
}
