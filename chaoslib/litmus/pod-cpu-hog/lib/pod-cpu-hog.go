package lib

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-cpu-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

// StressCPU Uses the REST API to exec into the target container of the target pod
// The function will be constantly increasing the CPU utilisation until it reaches the maximum available or allowed number.
// Using the TOTAL_CHAOS_DURATION we will need to specify for how long this experiment will last
func StressCPU(experimentsDetails *experimentTypes.ExperimentDetails, podName string, clients clients.ClientSets) error {
	// It will contains all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}
	command := []string{"/bin/sh", "-c", experimentsDetails.ChaosInjectCmd}
	litmusexec.SetExecCommandAttributes(&execCommandDetails, podName, experimentsDetails.TargetContainer, experimentsDetails.AppNS)
	_, err := litmusexec.Exec(&execCommandDetails, clients, command)
	if err != nil {
		return errors.Errorf("Unable to run stress command inside target container, err: %v", err)
	}

	return nil
}

//ExperimentCPU function orchestrates the experiment by calling the StressCPU function for every core, of every container, of every pod that is targeted
func ExperimentCPU(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
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
		experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, targetPodList.Items[0].Name, clients)
		if err != nil {
			return errors.Errorf("Unable to get the target container name, err: %v", err)
		}
	}

	if experimentsDetails.Sequence == "serial" {
		if err = InjectChaosInSerialMode(experimentsDetails, targetPodList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	} else {
		if err = InjectChaosInParallelMode(experimentsDetails, targetPodList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	}

	return nil
}

// InjectChaosInSerialMode stressed the cpu of all target application serially (one by one)
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList corev1.PodList, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	for _, pod := range targetPodList.Items {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		log.InfoWithValues("[Chaos]: The Target application details", logrus.Fields{
			"Target Container": experimentsDetails.TargetContainer,
			"Target Pod":       pod.Name,
			"CPU CORE":         experimentsDetails.CPUcores,
		})

		for i := 0; i < experimentsDetails.CPUcores; i++ {
			go StressCPU(experimentsDetails, pod.Name, clients)
		}

		log.Infof("[Chaos]:Waiting for: %vs", experimentsDetails.ChaosDuration)

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
				err := KillStressCPUSerial(experimentsDetails, pod.Name, clients)
				if err != nil {
					klog.V(0).Infof("Error in Kill stress after abortion")
					return err
				}
				// updating the chaosresult after stopped
				failStep := "CPU hog Chaos injection stopped!"
				types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
				result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

				// generating summary event in chaosengine
				msg := experimentsDetails.ExperimentName + " experiment has been aborted"
				types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

				// generating summary event in chaosresult
				types.SetResultEventAttributes(eventsDetails, types.StoppedVerdict, msg, "Warning", resultDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")
				os.Exit(1)
			case <-endTime:
				log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
				endTime = nil
				break loop
			}
		}
		if err := KillStressCPUSerial(experimentsDetails, pod.Name, clients); err != nil {
			return err
		}
	}
	return nil
}

// InjectChaosInParallelMode stressed the cpu of all target application in parallel mode (all at once)
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList corev1.PodList, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	for _, pod := range targetPodList.Items {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		log.InfoWithValues("[Chaos]: The Target application details", logrus.Fields{
			"Target Container": experimentsDetails.TargetContainer,
			"Target Pod":       pod.Name,
			"CPU CORE":         experimentsDetails.CPUcores,
		})
		for i := 0; i < experimentsDetails.CPUcores; i++ {
			go StressCPU(experimentsDetails, pod.Name, clients)
		}
	}

	log.Infof("[Chaos]:Waiting for: %vs", experimentsDetails.ChaosDuration)

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
			err := KillStressCPUParallel(experimentsDetails, targetPodList, clients)
			if err != nil {
				klog.V(0).Infof("Error in Kill stress after abortion")
				return err
			}
			// updating the chaosresult after stopped
			failStep := "CPU hog Chaos injection stopped!"
			types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
			result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

			// generating summary event in chaosengine
			msg := experimentsDetails.ExperimentName + " experiment has been aborted"
			types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

			// generating summary event in chaosresult
			types.SetResultEventAttributes(eventsDetails, types.StoppedVerdict, msg, "Warning", resultDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")
			os.Exit(1)
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			endTime = nil
			break loop
		}
	}
	if err := KillStressCPUParallel(experimentsDetails, targetPodList, clients); err != nil {
		return err
	}

	return nil
}

//PrepareCPUstress contains the steps for prepration before chaos
func PrepareCPUstress(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	//Starting the CPU stress experiment
	err := ExperimentCPU(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails)
	if err != nil {
		return err
	}
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//GetTargetContainer will fetch the container name from application pod
// It will return the first container name from the application pod
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", err
	}

	return pod.Spec.Containers[0].Name, nil
}

// KillStressCPUSerial function to kill a stress process running inside target container
//  Triggered by either timeout of chaos duration or termination of the experiment
func KillStressCPUSerial(experimentsDetails *experimentTypes.ExperimentDetails, podName string, clients clients.ClientSets) error {
	// It will contains all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}

	command := []string{"/bin/sh", "-c", experimentsDetails.ChaosKillCmd}

	litmusexec.SetExecCommandAttributes(&execCommandDetails, podName, experimentsDetails.TargetContainer, experimentsDetails.AppNS)
	_, err := litmusexec.Exec(&execCommandDetails, clients, command)
	if err != nil {
		return errors.Errorf("Unable to kill the stress process in %v pod, err: %v", podName, err)
	}

	return nil
}

// KillStressCPUParallel function to kill all the stress process running inside target container
// Triggered by either timeout of chaos duration or termination of the experiment
func KillStressCPUParallel(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList corev1.PodList, clients clients.ClientSets) error {

	for _, pod := range targetPodList.Items {

		if err := KillStressCPUSerial(experimentsDetails, pod.Name, clients); err != nil {
			return err
		}
	}
	return nil
}
