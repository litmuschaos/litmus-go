package lib

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-memory-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	core_v1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

var err error

// StressMemory Uses the REST API to exec into the target container of the target pod
// The function will be constantly increasing the Memory utilisation until it reaches the maximum available or allowed number.
// Using the TOTAL_CHAOS_DURATION we will need to specify for how long this experiment will last
func StressMemory(MemoryConsumption, containerName, podName, namespace string, clients clients.ClientSets) error {

	log.Infof("The memory consumption is: %v", MemoryConsumption)

	// It will contain all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}

	ddCmd := fmt.Sprintf("dd if=/dev/zero of=/dev/null bs=" + MemoryConsumption + "M")
	command := []string{"/bin/sh", "-c", ddCmd}

	litmusexec.SetExecCommandAttributes(&execCommandDetails, podName, containerName, namespace)
	_, err := litmusexec.Exec(&execCommandDetails, clients, command)
	if err != nil {
		return errors.Errorf("Unable to run stress command inside target container, due to err: %v", err)
	}
	return nil
}

//ExperimentMemory function orchestrates the experiment by calling the StressMemory function, of every container, of every pod that is targeted
func ExperimentMemory(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	var realpods core_v1.PodList

	isPodAvailable, err := common.CheckForAvailibiltyOfPod(experimentsDetails.AppNS, experimentsDetails.TargetPod, clients)
	if err != nil {
		return err
	}

	if isPodAvailable {
		pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(experimentsDetails.TargetPod, v1.GetOptions{})
		if err != nil {
			return err
		}
		realpods.Items = append(realpods.Items, *pod)

	} else {
		log.Info("selecting a random pod with specified labels")
		realpods, err = PreparePodList(experimentsDetails, clients, resultDetails)
		if err != nil {
			return err
		}
	}

	for _, pod := range realpods.Items {

		for _, container := range pod.Status.ContainerStatuses {
			if container.Ready != true {
				return errors.Errorf("containers are not yet in running state")
			}
			log.InfoWithValues("The running status of container to stress is as follows", logrus.Fields{
				"container": container.Name, "Pod": pod.Name, "Status": pod.Status.Phase})

			log.Infof("[Chaos]:Stressing: %v Megabytes", strconv.Itoa(experimentsDetails.MemoryConsumption))

			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			go StressMemory(strconv.Itoa(experimentsDetails.MemoryConsumption), container.Name, pod.Name, experimentsDetails.AppNS, clients)

			log.Infof("[Chaos]:Waiting for: %vs", strconv.Itoa(experimentsDetails.ChaosDuration))

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
					err = KillStressMemory(container.Name, pod.Name, experimentsDetails.AppNS, experimentsDetails.ChaosKillCmd, clients)
					if err != nil {
						klog.V(0).Infof("Error in Kill stress after")
						return err
					}
					// updating the chaosresult after stopped
					failStep := "Memory hog Chaos injection stopped!"
					types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
					result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

					// generating summary event in chaosengine
					msg := experimentsDetails.ExperimentName + " experiment has been aborted"
					types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
					events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

					// generating summary event in chaosresult
					types.SetResultEventAttributes(eventsDetails, types.Summary, msg, "Warning", resultDetails)
					events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")
					os.Exit(1)
				case <-endTime:
					log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
					break loop
				}
			}
			if err = KillStressMemory(container.Name, pod.Name, experimentsDetails.AppNS, experimentsDetails.ChaosKillCmd, clients); err != nil {
				return err
			}
		}
	}

	return nil
}

//PrepareMemoryStress contains the steps for prepration before chaos
func PrepareMemoryStress(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	//Starting the Memory stress experiment
	err := ExperimentMemory(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails)
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

//PreparePodList will also adjust the number of the target pods depending on the specified percentage in PODS_AFFECTED_PERC variable
func PreparePodList(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails) (core_v1.PodList, error) {

	log.Infof("[Chaos]:Pods percentage to affect is %v", strconv.Itoa(experimentsDetails.PodsAffectedPerc))

	//Getting the list of pods with the given labels and namespaces
	pods, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil {
		resultDetails.FailStep = "Getting the list of pods with the given labels and namespaces"
		return core_v1.PodList{}, err
	}

	//If the default value has changed, means that we are aiming for a subset of the pods.
	if experimentsDetails.PodsAffectedPerc != 100 {

		newPodlistLength := math.Maximum(1, math.Adjustment(experimentsDetails.PodsAffectedPerc, len(pods.Items)))

		pods.Items = pods.Items[:newPodlistLength]

		log.Infof("[Chaos]:Number of pods targeted: %v", strconv.Itoa(newPodlistLength))

	}
	return *pods, nil
}

//KillStressMemory function to kill the experiment. Triggered by either timeout of chaos duration or termination of the experiment
func KillStressMemory(containerName, podName, namespace, memFreeCmd string, clients clients.ClientSets) error {
	// It will contains all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}

	command := []string{"/bin/sh", "-c", memFreeCmd}

	litmusexec.SetExecCommandAttributes(&execCommandDetails, podName, containerName, namespace)
	_, err := litmusexec.Exec(&execCommandDetails, clients, command)
	if err != nil {
		return errors.Errorf("Unable to kill stress process inside target container, due to err: %v", err)
	}
	return nil
}
