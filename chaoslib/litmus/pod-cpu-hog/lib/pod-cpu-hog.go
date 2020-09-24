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
func StressCPU(containerName, podName, namespace, cpuHogCmd string, clients clients.ClientSets) error {
	// It will contains all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}
	command := []string{"/bin/sh", "-c", cpuHogCmd}
	litmusexec.SetExecCommandAttributes(&execCommandDetails, podName, containerName, namespace)
	_, err := litmusexec.Exec(&execCommandDetails, clients, command)
	if err != nil {
		return errors.Errorf("Unable to run stress command inside target container, err: %v", err)
	}
	return nil
}

//ExperimentCPU function orchestrates the experiment by calling the StressCPU function for every core, of every container, of every pod that is targeted
func ExperimentCPU(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	targetPodList, err := common.GetPodList(experimentsDetails.AppNS, experimentsDetails.TargetPod, experimentsDetails.AppLabel, experimentsDetails.PodsAffectedPerc, clients)
	if err != nil {
		return errors.Errorf("Unable to get the target pod list, err: %v", err)
	}

	//Get the target container name of the application pod
	if experimentsDetails.TargetContainer == "" {
		experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, targetPodList.Items[0].Name, clients)
		if err != nil {
			return errors.Errorf("Unable to get the target container name, err: %v", err)
		}
	}

	for _, pod := range targetPodList.Items {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		log.InfoWithValues("[Chaos]: The Target application details", logrus.Fields{
			"container": experimentsDetails.TargetContainer,
			"Pod":       pod.Name,
			"CPU CORE":  experimentsDetails.CPUcores,
		})

		for i := 0; i < experimentsDetails.CPUcores; i++ {

			go StressCPU(experimentsDetails.TargetContainer, pod.Name, experimentsDetails.AppNS, experimentsDetails.ChaosInjectCmd, clients)

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
			err = KillStressCPU(experimentsDetails.TargetContainer, experimentsDetails.AppNS, experimentsDetails.ChaosKillCmd, clients, targetPodList)
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
			break loop
		}
	}
	if err = KillStressCPU(experimentsDetails.TargetContainer, experimentsDetails.AppNS, experimentsDetails.ChaosKillCmd, clients, targetPodList); err != nil {
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

// KillStressCPU function to kill the experiment. Triggered by either timeout of chaos duration or termination of the experiment
func KillStressCPU(containerName, namespace, cpuFreeCmd string, clients clients.ClientSets, targetPodList corev1.PodList) error {
	// It will contains all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}

	command := []string{"/bin/sh", "-c", cpuFreeCmd}

	for _, pod := range targetPodList.Items {

		litmusexec.SetExecCommandAttributes(&execCommandDetails, pod.Name, containerName, namespace)
		_, err := litmusexec.Exec(&execCommandDetails, clients, command)
		if err != nil {
			return errors.Errorf("Unable to kill the stress process in %v pod, err: %v", pod.Name, err)
		}
	}
	return nil
}
