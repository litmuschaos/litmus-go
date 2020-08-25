package lib

import (
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-cpu-hog/types"
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

// StressCPU Uses the REST API to exec into the target container of the target pod
// The function will be constantly increasing the CPU utilisation until it reaches the maximum available or allowed number.
// Using the TOTAL_CHAOS_DURATION we will need to specify for how long this experiment will last
func StressCPU(containerName, podName, namespace string, clients clients.ClientSets) error {
	// It will contains all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}
	command := []string{"/bin/sh", "-c", "md5sum /dev/zero"}
	litmusexec.SetExecCommandAttributes(&execCommandDetails, podName, containerName, namespace)
	_, err := litmusexec.Exec(&execCommandDetails, clients, command)
	if err != nil {
		return errors.Errorf("Unable to run stress command inside target container, due to err: %v", err)
	}
	return nil
}

//ExperimentCPU function orchestrates the experiment by calling the StressCPU function for every core, of every container, of every pod that is targeted
func ExperimentCPU(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	realpods := core_v1.PodList{}

	// checking for the availibilty of the target pod
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
		// selecting the random pod, if the target pod is not specified
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

			log.Infof("[Chaos]:Stressing: %v cores", strconv.Itoa(experimentsDetails.CPUcores))

			for i := 0; i < experimentsDetails.CPUcores; i++ {

				if experimentsDetails.EngineName != "" {
					msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
					types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
					events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
				}

				go StressCPU(container.Name, pod.Name, experimentsDetails.AppNS, clients)

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
						err = KillStressCPU(container.Name, pod.Name, experimentsDetails.AppNS, clients)
						if err != nil {
							klog.V(0).Infof("Error in Kill stress after")
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
						types.SetResultEventAttributes(eventsDetails, types.Summary, msg, "Warning", resultDetails)
						events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")
						os.Exit(1)
					case <-endTime:
						log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
						break loop
					}
				}
				err = KillStressCPU(container.Name, pod.Name, experimentsDetails.AppNS, clients)
				if err != nil {
					errorCode := strings.Contains(err.Error(), "143")
					if errorCode != true {
						log.Infof("[Chaos]:CPU stress error: %v", err.Error())
						return err
					}
				}
			}
		}
	}

	return nil
}

//PrepareCPUstress contains the steps for prepration before chaos
func PrepareCPUstress(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	//Starting the CPU stress experiment
	err := ExperimentCPU(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails)
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

		newPodListLength := math.Maximum(1, math.Adjustment(experimentsDetails.PodsAffectedPerc, len(pods.Items)))

		pods.Items = pods.Items[:newPodListLength]

		log.Infof("[Chaos]:Number of pods targeted: %v", strconv.Itoa(newPodListLength))

	}
	return *pods, nil
}

// KillStressCPU function to kill the experiment. Triggered by either timeout of chaos duration or termination of the experiment
func KillStressCPU(containerName, podName, namespace string, clients clients.ClientSets) error {
	// It will contains all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}

	command := []string{"/bin/sh", "-c", "kill $(find /proc -name exe -lname '*/md5sum' 2>&1 | grep -v 'Permission denied' | awk -F/ '{print $(NF-1)}' |  head -n 1)"}

	litmusexec.SetExecCommandAttributes(&execCommandDetails, podName, containerName, namespace)
	_, err := litmusexec.Exec(&execCommandDetails, clients, command)
	if err != nil {
		return errors.Errorf("Unable to kill the stress process, due to err: %v", err)
	}
	return nil
}
