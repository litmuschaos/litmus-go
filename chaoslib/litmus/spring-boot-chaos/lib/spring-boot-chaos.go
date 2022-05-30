package lib

import (
	"bytes"
	"encoding/json"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/spring-boot/spring-boot-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// SetTargetPodList selects the targeted pod and add them to the experimentDetails
func SetTargetPodList(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {
	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage

	if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail.Label == "" {
		return errors.Errorf("please provide one of the appLabel or TARGET_PODS")
	}

	if experimentsDetails.TargetPodList, err := common.GetPodList(experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails); err != nil {
		return err
	}
	return nil

}

// PrepareChaos contains the preparation steps before chaos injection
func PrepareChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err := injectChaosInSerialMode(experimentsDetails, clients, chaosDetails, eventsDetails, resultDetails); err != nil {
			return err
		}
	case "parallel":
		if err := injectChaosInParallelMode(experimentsDetails, clients, chaosDetails, eventsDetails, resultDetails); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
	}

	// Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// CheckChaosMonkey verifies if chaos monkey for spring boot is available in the selected pods
func CheckChaosMonkey(chaosMonkeyPort string, chaosMonkeyPath string, targetPods corev1.PodList) (bool, error) {
	// Deleting the application pod
	for _, pod := range targetPods.Items {
		log.Infof("[Check]: Checking pod: %v", pod.Name)
		if resp, err := http.Get("http://" + pod.Status.PodIP + ":" + chaosMonkeyPort + chaosMonkeyPath) err != nil {.   //nolint:bodyclose
			return false, err
		}

		if resp.StatusCode != 200 {
			return false, errors.Errorf("failed to get chaos monkey endpoint on pod %v (status: %v)", pod.Name, resp.StatusCode)
		}
	}

	return true, nil
}

// enableChaosMonkey enables chaos monkey on selected pods
func enableChaosMonkey(chaosMonkeyPort string, chaosMonkeyPath string, pod corev1.Pod) error {
	log.Infof("[Chaos]: Enabling Chaos Monkey on pod: %v", pod.Name)
	resp, err := http.Post("http://"+pod.Status.PodIP+":"+chaosMonkeyPort+chaosMonkeyPath+"/enable", "", nil) //nolint:bodyclose
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("failed to enable chaos monkey endpoint on pod %v (status: %v)", pod.Name, resp.StatusCode)
	}

	return nil
}

func setChaosMonkeyWatchers(chaosMonkeyPort string, chaosMonkeyPath string, watchers experimentTypes.ChaosMonkeyWatchers, pod corev1.Pod) error {
	log.Infof("[Check]: Setting Chaos Monkey watchers on pod: %v", pod.Name)

	jsonValue, err := json.Marshal(watchers)
	if err != nil {
		return err
	}

	resp, err := http.Post("http://"+pod.Status.PodIP+":"+chaosMonkeyPort+chaosMonkeyPath+"/watchers", "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("failed to set assault on pod %v (status: %v)", pod.Name, resp.StatusCode)
	}

	return nil
}

func setChaosMonkeyAssault(chaosMonkeyPort string, chaosMonkeyPath string, assault experimentTypes.ChaosMonkeyAssault, pod corev1.Pod) error {
	log.Infof("[Check]: Setting Chaos Monkey assault on pod: %v", pod.Name)

	jsonValue, err := json.Marshal(assault)
	if err != nil {
		return err
	}

	resp, err := http.Post("http://"+pod.Status.PodIP+":"+chaosMonkeyPort+chaosMonkeyPath+"/assaults", "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("failed to set assault on pod %v (status: %v)", pod.Name, resp.StatusCode)
	}

	log.Infof("[Check]: Activating Chaos Monkey assault on pod: %v", pod.Name)
	resp, err = http.Post("http://"+pod.Status.PodIP+":"+chaosMonkeyPort+chaosMonkeyPath+"/assaults/runtime/attack", "", nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("failed to activate runtime attack on pod %v (status: %v)", pod.Name, resp.StatusCode)
	}

	return nil
}

// disableChaosMonkey disables chaos monkey on selected pods
func disableChaosMonkey(chaosMonkeyPort string, chaosMonkeyPath string, pod corev1.Pod) error {
	log.Infof("[Check]: disabling chaos monkey on pods %v", pod.Name)
	resp, err := http.Post("http://"+pod.Status.PodIP+":"+chaosMonkeyPort+chaosMonkeyPath+"/disable", "", nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("failed to disable chaos monkey endpoint on pod %v (status: %v)", pod.Name, resp.StatusCode)
	}

	return nil
}

// injectChaosInSerialMode delete the target application pods serial mode(one by one)
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails, resultDetails *types.ResultDetails) error {

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
	case <-signChan:
		// stopping the chaos execution, if abort signal received
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		for _, pod := range experimentsDetails.TargetPodList.Items {
			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				_ = events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			log.InfoWithValues("[Chaos]: The Target application details", logrus.Fields{
				"Target Pod": pod.Name,
			})

			if err := setChaosMonkeyWatchers(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, experimentsDetails.ChaosMonkeyWatchers, pod); err != nil {
				log.Errorf("[Chaos]: Failed to set watchers, err: %v ", err)
				return err
			}

			if err := setChaosMonkeyAssault(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, experimentsDetails.ChaosMonkeyAssault, pod); err != nil {
				log.Errorf("[Chaos]: Failed to set assault, err: %v ", err)
				return err
			}

			if err := enableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
				log.Errorf("[Chaos]: Failed to enable chaos, err: %v ", err)
				return err
			}
			common.SetTargets(pod.Name, "injected", "pod", chaosDetails)

			log.Infof("[Chaos]:Waiting for: %vs", experimentsDetails.ChaosDuration)

			endTime = time.After(timeDelay)
		loop:
			for {
				select {
				case <-signChan:
					log.Info("[Chaos]: Revert Started")
					if err := disableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
						log.Errorf("Error in disabling chaos monkey, err: %v", err)
					}
					common.SetTargets(pod.Name, "reverted", "pod", chaosDetails)
					os.Exit(1)
				case <-endTime:
					log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
					endTime = nil
					break loop
				}
			}
			if err := disableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
				log.Errorf("Error in disable chaos monkey, err: %v", err)
			}
		}
	}
	return nil

}

// injectChaosInParallelMode delete the target application pods in parallel mode (all at once)
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails, resultDetails *types.ResultDetails) error {

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
	case <-signChan:
		// stopping the chaos execution, if abort signal received
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		for _, pod := range experimentsDetails.TargetPodList.Items {
			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				_ = events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}

			log.InfoWithValues("[Chaos]: The Target application details", logrus.Fields{
				"Target Pod": pod.Name,
			})

			if err := setChaosMonkeyWatchers(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, experimentsDetails.ChaosMonkeyWatchers, pod); err != nil {
				return errors.Errorf("[Chaos]: Failed to set watchers, err: %v ", 
			}

			if err := setChaosMonkeyAssault(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, experimentsDetails.ChaosMonkeyAssault, pod); err != nil {
				log.Errorf("[Chaos]: Failed to set assault, err: %v ", err)
				return err
			}

			if err := enableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
				log.Errorf("[Chaos]: Failed to enable chaos, err: %v ", err)
				return err
			}
			common.SetTargets(pod.Name, "injected", "pod", chaosDetails)
		}
		log.Infof("[Chaos]:Waiting for: %vs", experimentsDetails.ChaosDuration)
	}
loop:
	for {
		endTime = time.After(timeDelay)
		select {
		case <-signChan:
			log.Info("[Chaos]: Revert Started")
			for _, pod := range experimentsDetails.TargetPodList.Items {
				if err := disableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
					log.Errorf("Error in disable chaos monkey, err: %v", err)
				}
				common.SetTargets(pod.Name, "reverted", "pod", chaosDetails)
			}
			os.Exit(1)
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			endTime = nil
			break loop
		}
	}
	for _, pod := range experimentsDetails.TargetPodList.Items {
		if err := disableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
			log.Errorf("Error in disable chaos monkey, err: %v", err)
		}
	}
	return nil
}
