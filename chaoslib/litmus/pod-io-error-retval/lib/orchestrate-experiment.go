package lib

import (
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-memory-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExperimentOrchestrationDetails bundles together all details connected to the orchestration
// of and experiment.
type ExperimentOrchestrationDetails struct {
	ExperimentDetails *experimentTypes.ExperimentDetails
	Clients           clients.ClientSets
	ResultDetails     *types.ResultDetails
	EventDetails      *types.EventDetails
	ChaosDetails      *types.ChaosDetails
	TargetPodList     corev1.PodList
}

// TargetContainer returns the targeted container in the targeted pod
func TargetContainer(exp ExperimentOrchestrationDetails, appName string) (string, error) {
	pod, err := exp.Clients.KubeClient.CoreV1().
		Pods(exp.ExperimentDetails.AppNS).
		Get(appName, v1.GetOptions{})
	targetContainer := ""

	if err == nil {
		targetContainer = pod.Spec.Containers[0].Name
	}

	return targetContainer, err
}

// ChaosInjector is and interface for abstracting all chaos injection mechanisms
type ChaosInjector interface {
	InjectChaosInSerialMode(
		exp ExperimentOrchestrationDetails) error
	InjectChaosInParallelMode(
		exp ExperimentOrchestrationDetails) error
}

// safeExperiment is an abstraction for lifting errors while orchestrating experiments
type safeExperiment struct {
	experiment ExperimentOrchestrationDetails
	err        error
}

func (exp *safeExperiment) verifyAppLabelOrTargetPodSpecified() {
	if exp.err != nil {
		return
	}

	targetPods := exp.experiment.ExperimentDetails.TargetPods
	appLabel := exp.experiment.ChaosDetails.AppDetail.Label

	if targetPods == "" && appLabel == "" {
		exp.err = errors.Errorf("Please provide one of appLabel or TARGET_PODS")
	}
}

func (exp *safeExperiment) obtainTargetPods() {
	if exp.err != nil {
		return
	}

	exp.experiment.TargetPodList, exp.err = common.GetPodList(
		exp.experiment.ExperimentDetails.TargetPods,
		exp.experiment.ExperimentDetails.PodsAffectedPerc,
		exp.experiment.Clients,
		exp.experiment.ChaosDetails,
	)
}

func (exp *safeExperiment) logTargetPodNames() {
	if exp.err != nil {
		return
	}

	podNames := []string{}
	for _, pod := range exp.experiment.TargetPodList.Items {
		podNames = append(podNames, pod.Name)
	}

	log.Infof("Target pods list for chaos, %v", podNames)
}

func (exp *safeExperiment) obtainTargetContainer() {
	if exp.err != nil {
		return
	}

	experimentDetails := exp.experiment.ExperimentDetails
	if experimentDetails.TargetContainer != "" {
		return
	}

	experimentDetails.TargetContainer, exp.err = TargetContainer(
		exp.experiment, exp.experiment.TargetPodList.Items[0].Name)

	if exp.err != nil {
		exp.err = errors.Errorf(
			"Unable to get the target container name, err: %v", exp.err)
	}
}

func (exp *safeExperiment) injectChaos(chaosInjector ChaosInjector) {
	if exp.err != nil {
		return
	}

	switch exp.experiment.ExperimentDetails.Sequence {
	case "serial":
		exp.err = chaosInjector.InjectChaosInSerialMode(exp.experiment)
	default:
		exp.err = chaosInjector.InjectChaosInParallelMode(exp.experiment)
	}
}

func (exp *safeExperiment) waitForRampTimeDuration(sequence string) {
	rampTime := exp.experiment.ExperimentDetails.RampTime
	if exp.err != nil || rampTime == 0 {
		return
	}

	log.Infof("[Ramp]: Waiting for the %vs ramp time %s injecting chaos",
		rampTime, sequence)
	common.WaitForDuration(rampTime)
}

// OrchestrateExperiment orchestrates a new chaos experiment with the given experiment details
// and the ChaosInjector for the chaos injection mechanism.
func OrchestrateExperiment(exp ExperimentOrchestrationDetails, chaosInjector ChaosInjector) error {
	safeExperimentOrchestrator := safeExperiment{experiment: exp}

	safeExperimentOrchestrator.waitForRampTimeDuration("before")

	safeExperimentOrchestrator.verifyAppLabelOrTargetPodSpecified()
	safeExperimentOrchestrator.obtainTargetPods()
	safeExperimentOrchestrator.logTargetPodNames()
	safeExperimentOrchestrator.obtainTargetContainer()
	safeExperimentOrchestrator.injectChaos(chaosInjector)

	safeExperimentOrchestrator.waitForRampTimeDuration("after")

	return safeExperimentOrchestrator.err
}
