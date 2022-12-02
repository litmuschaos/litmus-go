package lib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
	corev1 "k8s.io/api/core/v1"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/spring-boot/spring-boot-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
)

var revertAssault = experimentTypes.ChaosMonkeyAssaultRevert{
	LatencyActive:         false,
	KillApplicationActive: false,
	CPUActive:             false,
	MemoryActive:          false,
	ExceptionsActive:      false,
}

// SetTargetPodList selects the targeted pod and add them to the experimentDetails
func SetTargetPodList(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {
	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	var err error

	if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail == nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "please provide one of the appLabel or TARGET_PODS"}
	}
	if experimentsDetails.TargetPodList, err = common.GetPodList(experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails); err != nil {
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

	log.InfoWithValues("[Info]: Chaos monkeys watchers will be injected to the target pods as follows", logrus.Fields{
		"WebClient":      experimentsDetails.ChaosMonkeyWatchers.WebClient,
		"Service":        experimentsDetails.ChaosMonkeyWatchers.Service,
		"Component":      experimentsDetails.ChaosMonkeyWatchers.Component,
		"Repository":     experimentsDetails.ChaosMonkeyWatchers.Repository,
		"Controller":     experimentsDetails.ChaosMonkeyWatchers.Controller,
		"RestController": experimentsDetails.ChaosMonkeyWatchers.RestController,
	})

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err := injectChaosInSerialMode(experimentsDetails, clients, chaosDetails, eventsDetails, resultDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err := injectChaosInParallelMode(experimentsDetails, clients, chaosDetails, eventsDetails, resultDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
	}

	// Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// CheckChaosMonkey verifies if chaos monkey for spring boot is available in the selected pods
// All pods are checked, even if some errors occur. But in case of one pod in error, the check will be in error
func CheckChaosMonkey(chaosMonkeyPort string, chaosMonkeyPath string, targetPods corev1.PodList) (bool, error) {
	hasErrors := false

	targetPodNames := []string{}

	for _, pod := range targetPods.Items {

		targetPodNames = append(targetPodNames, pod.Name)

		endpoint := "http://" + pod.Status.PodIP + ":" + chaosMonkeyPort + chaosMonkeyPath
		log.Infof("[Check]: Checking pod: %v (endpoint: %v)", pod.Name, endpoint)

		resp, err := http.Get(endpoint)
		if err != nil {
			log.Errorf("failed to request chaos monkey endpoint on pod %s, %s", pod.Name, err.Error())
			hasErrors = true
			continue
		}

		if resp.StatusCode != 200 {
			log.Errorf("failed to get chaos monkey endpoint on pod %s (status: %d)", pod.Name, resp.StatusCode)
			hasErrors = true
		}
	}

	if hasErrors {
		return false, cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{podNames: %s}", targetPodNames), Reason: "failed to check chaos monkey on at least one pod, check logs for details"}
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
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to enable chaos monkey endpoint (status: %d)", resp.StatusCode)}
	}

	return nil
}

func setChaosMonkeyWatchers(chaosMonkeyPort string, chaosMonkeyPath string, watchers experimentTypes.ChaosMonkeyWatchers, pod corev1.Pod) error {
	log.Infof("[Chaos]: Setting Chaos Monkey watchers on pod: %v", pod.Name)

	jsonValue, err := json.Marshal(watchers)
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to marshal chaos monkey watchers, %s", err.Error())}
	}

	resp, err := http.Post("http://"+pod.Status.PodIP+":"+chaosMonkeyPort+chaosMonkeyPath+"/watchers", "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to call the chaos monkey api to set watchers, %s", err.Error())}
	}

	if resp.StatusCode != 200 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to set assault (status: %d)", resp.StatusCode)}
	}

	return nil
}

func startAssault(chaosMonkeyPort string, chaosMonkeyPath string, assault []byte, pod corev1.Pod) error {
	if err := setChaosMonkeyAssault(chaosMonkeyPort, chaosMonkeyPath, assault, pod); err != nil {
		return err
	}
	log.Infof("[Chaos]: Activating Chaos Monkey assault on pod: %v", pod.Name)
	resp, err := http.Post("http://"+pod.Status.PodIP+":"+chaosMonkeyPort+chaosMonkeyPath+"/assaults/runtime/attack", "", nil)
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to call the chaos monkey api to start assault %s", err.Error())}
	}

	if resp.StatusCode != 200 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to activate runtime attack (status: %d)", resp.StatusCode)}
	}
	return nil
}

func setChaosMonkeyAssault(chaosMonkeyPort string, chaosMonkeyPath string, assault []byte, pod corev1.Pod) error {
	log.Infof("[Chaos]: Setting Chaos Monkey assault on pod: %v", pod.Name)

	resp, err := http.Post("http://"+pod.Status.PodIP+":"+chaosMonkeyPort+chaosMonkeyPath+"/assaults", "application/json", bytes.NewBuffer(assault))
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to call the chaos monkey api to set assault, %s", err.Error())}
	}

	if resp.StatusCode != 200 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to set assault (status: %d)", resp.StatusCode)}
	}
	return nil
}

// disableChaosMonkey disables chaos monkey on selected pods
func disableChaosMonkey(chaosMonkeyPort string, chaosMonkeyPath string, pod corev1.Pod) error {
	log.Infof("[Chaos]: disabling assaults on pod %s", pod.Name)
	jsonValue, err := json.Marshal(revertAssault)
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to marshal chaos monkey revert-chaos watchers, %s", err.Error())}
	}
	if err := setChaosMonkeyAssault(chaosMonkeyPort, chaosMonkeyPath, jsonValue, pod); err != nil {
		return err
	}

	log.Infof("[Chaos]: disabling chaos monkey on pod %s", pod.Name)
	resp, err := http.Post("http://"+pod.Status.PodIP+":"+chaosMonkeyPort+chaosMonkeyPath+"/disable", "", nil)
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to call the chaos monkey api to disable assault, %s", err.Error())}
	}

	if resp.StatusCode != 200 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{podName: %s, namespace: %s}", pod.Name, pod.Namespace), Reason: fmt.Sprintf("failed to disable chaos monkey endpoint (status: %d)", resp.StatusCode)}
	}

	return nil
}

// injectChaosInSerialMode injects chaos monkey assault on pods in serial mode(one by one)
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

			log.InfoWithValues("[Chaos]: Injecting on target pod", logrus.Fields{
				"Target Pod": pod.Name,
			})

			if err := setChaosMonkeyWatchers(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, experimentsDetails.ChaosMonkeyWatchers, pod); err != nil {
				log.Errorf("[Chaos]: Failed to set watchers, err: %v ", err)
				return err
			}

			if err := startAssault(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, experimentsDetails.ChaosMonkeyAssault, pod); err != nil {
				log.Errorf("[Chaos]: Failed to set assault, err: %v ", err)
				return err
			}

			if err := enableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
				log.Errorf("[Chaos]: Failed to enable chaos, err: %v ", err)
				return err
			}
			common.SetTargets(pod.Name, "injected", "pod", chaosDetails)

			log.Infof("[Chaos]: Waiting for: %vs", experimentsDetails.ChaosDuration)

			endTime = time.After(timeDelay)
		loop:
			for {
				select {
				case <-signChan:
					log.Info("[Chaos]: Revert Started")
					if err := disableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
						log.Errorf("Error in disabling chaos monkey, err: %v", err)
					} else {
						common.SetTargets(pod.Name, "reverted", "pod", chaosDetails)
					}
					// updating the chaosresult after stopped
					failStep := "Chaos injection stopped!"
					types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep, cerrors.ErrorTypeExperimentAborted)
					result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")
					log.Info("[Chaos]: Revert Completed")
					os.Exit(1)
				case <-endTime:
					log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
					endTime = nil
					break loop
				}
			}

			if err := disableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
				return err
			}

			common.SetTargets(pod.Name, "reverted", "pod", chaosDetails)
		}
	}
	return nil

}

// injectChaosInParallelMode injects chaos monkey assault on pods in parallel mode (all at once)
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
				log.Errorf("[Chaos]: Failed to set watchers, err: %v", err)
				return err
			}

			if err := startAssault(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, experimentsDetails.ChaosMonkeyAssault, pod); err != nil {
				log.Errorf("[Chaos]: Failed to set assault, err: %v", err)
				return err
			}

			if err := enableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
				log.Errorf("[Chaos]: Failed to enable chaos, err: %v", err)
				return err
			}
			common.SetTargets(pod.Name, "injected", "pod", chaosDetails)
		}
		log.Infof("[Chaos]: Waiting for: %vs", experimentsDetails.ChaosDuration)
	}
loop:
	for {
		endTime = time.After(timeDelay)
		select {
		case <-signChan:
			log.Info("[Chaos]: Revert Started")
			for _, pod := range experimentsDetails.TargetPodList.Items {
				if err := disableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
					log.Errorf("Error in disabling chaos monkey, err: %v", err)
				} else {
					common.SetTargets(pod.Name, "reverted", "pod", chaosDetails)
				}
			}
			// updating the chaosresult after stopped
			failStep := "Chaos injection stopped!"
			types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep, cerrors.ErrorTypeExperimentAborted)
			result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")
			log.Info("[Chaos]: Revert Completed")
			os.Exit(1)
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			endTime = nil
			break loop
		}
	}

	var errorList []string
	for _, pod := range experimentsDetails.TargetPodList.Items {
		if err := disableChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, pod); err != nil {
			errorList = append(errorList, err.Error())
			continue
		}
		common.SetTargets(pod.Name, "reverted", "pod", chaosDetails)
	}

	if len(errorList) != 0 {
		return cerrors.PreserveError{ErrString: fmt.Sprintf("error in disabling chaos monkey, [%s]", strings.Join(errorList, ","))}
	}
	return nil
}
