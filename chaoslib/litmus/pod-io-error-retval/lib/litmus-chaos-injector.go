// Package pod-io-error-retval/lib is a library for orchestrating
// io-error chaos experiments based on the debugfs fail function feature.
package lib

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/litmuschaos/litmus-go/pkg/log"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-memory-hog/types"

	corev1 "k8s.io/api/core/v1"
)

// ChaosInjectorFn represents first order function for injecting chaos with the given executor.
// The chaos ia parameterized and is initialized with the provided chaos parameters. Any errors
// resulting from the injection of the chaos is emitted to the provided erro channel.
type ChaosInjectorFunction func(executor Executor, chaosParams interface{}, errChannel chan error)

// ResetChaosFunction represents first order function for resetting chaos with the given executor.
type ResetChaosFunction func(Executor Executor, chaosParams interface{}) error

// LitmusChaosParamFunction Parses chaos parameters from experiment details.
type LitmusChaosParamFunction func(exp *experimentTypes.ExperimentDetails) interface{}

// LitmusChaosInjector
type LitmusChaosInjector struct {
	ChaosParamsFn   LitmusChaosParamFunction
	ChaosInjectorFn ChaosInjectorFunction
	ResetChaosFn    ResetChaosFunction
}

// InjectChaosInSerialMode injects chaos with the given experiment details in serial mode.
func (injector LitmusChaosInjector) InjectChaosInSerialMode(exp ExperimentOrchestrationDetails) error {
	orchestrator := litmusChaosOrchestrator{
		errChannel:    make(chan error),
		signalChannel: make(chan os.Signal, 1),
		injector:      injector,
		exp:           exp,
	}

	orchestrator.runProbes()

	for _, pod := range exp.TargetPodList.Items {
		orchestrator.obtainChaosParams()
		orchestrator.generateChaosEventsOnPod(pod)
		orchestrator.logChaosDetails(pod)
		orchestrator.orchestrateChaos(pod)

		log.Infof("[Chaos]:Waiting for: %vs", exp.ExperimentDetails.ChaosDuration)

		// Trap os.Interrupt and syscall.SIGTERM an emit a value on the signal channel. Note
		// that syscall.SIGKILL cannot be trapped.
		signal.Notify(orchestrator.signalChannel, os.Interrupt, syscall.SIGTERM) // , syscall.SIGKILL)

		orchestrator.observeAndReact([]corev1.Pod{pod})
	}

	return orchestrator.err
}

// InjectChaosInParallelMode injects chaos with the given experiment details in parallel mode.
func (injector LitmusChaosInjector) InjectChaosInParallelMode(exp ExperimentOrchestrationDetails) error {
	orchestrator := litmusChaosOrchestrator{
		errChannel:    make(chan error),
		signalChannel: make(chan os.Signal, 1),
		injector:      injector,
		exp:           exp,
	}

	orchestrator.runProbes()

	for _, pod := range exp.TargetPodList.Items {
		orchestrator.obtainChaosParams()
		orchestrator.generateChaosEventsOnPod(pod)
		orchestrator.logChaosDetails(pod)
		orchestrator.orchestrateChaos(pod)
	}

	log.Infof("[Chaos]:Waiting for: %vs", exp.ExperimentDetails.ChaosDuration)

	// Trap os.Interrupt and syscall.SIGTERM an emit a value on the signal channel. Note
	// that syscall.SIGKILL cannot be trapped.
	signal.Notify(orchestrator.signalChannel, os.Interrupt, syscall.SIGTERM) // , syscall.SIGKILL)

	orchestrator.observeAndReact(exp.TargetPodList.Items)

	return orchestrator.err
}
