package lib

import (
	"fmt"
	"os"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// litmusChaosOrchestrator is an abstraction for maintaining orchestration state and lifting errors.
type litmusChaosOrchestrator struct {
	errChannel    chan error
	endTime       <-chan time.Time
	signalChannel chan os.Signal
	err           error

	injector    LitmusChaosInjector
	exp         ExperimentOrchestrationDetails
	chaosParams interface{}
}

func (c *litmusChaosOrchestrator) runProbes() {
	if c.err != nil || len(c.exp.ResultDetails.ProbeDetails) == 0 {
		return
	}

	c.err = probe.RunProbes(
		c.exp.ChaosDetails,
		c.exp.Clients,
		c.exp.ResultDetails,
		"During Chaos",
		c.exp.EventDetails,
	)
}

func (c *litmusChaosOrchestrator) startEndTimer() {
	if c.err != nil {
		return
	}

	chaosDuration := c.exp.ExperimentDetails.ChaosDuration
	duration := time.Duration(chaosDuration) * time.Second
	c.endTime = time.After(duration)
}

func (c *litmusChaosOrchestrator) generateChaosEventsOnPod(pod corev1.Pod) {
	if c.err != nil || c.exp.ExperimentDetails.EngineName == "" {
		return
	}

	msg := fmt.Sprintf("Injecting %s chaos on %s pod.",
		c.exp.ChaosDetails.ExperimentName,
		pod.Name)

	types.SetEngineEventAttributes(
		c.exp.EventDetails,
		types.ChaosInject,
		msg,
		"Normal",
		c.exp.ChaosDetails,
	)
}

func (c *litmusChaosOrchestrator) obtainChaosParams() {
	if c.err != nil {
		return
	}

	c.chaosParams = c.injector.ChaosParamsFn(
		c.exp.ExperimentDetails)
}

func (c *litmusChaosOrchestrator) logChaosDetails(pod corev1.Pod) {
	if c.err != nil {
		return
	}

	log.InfoWithValues("[Chaos] The target application details",
		logrus.Fields{
			"Target Container": c.exp.ExperimentDetails.TargetContainer,
			"Target Pod":       pod.Name,
			"Chaos Details":    fmt.Sprint(c.chaosParams),
		},
	)
}

func (c *litmusChaosOrchestrator) orchestrateChaos(pod corev1.Pod) {
	if c.err != nil {
		return
	}

	executor := LitmusExecutor{
		ContainerName: c.exp.ExperimentDetails.TargetContainer,
		PodName:       pod.Name,
		Namespace:     c.exp.ExperimentDetails.AppNS,
		Clients:       c.exp.Clients,
	}

	go c.injector.ChaosInjectorFn(
		executor, c.chaosParams, c.errChannel)
}

func (c *litmusChaosOrchestrator) killChaos(pod corev1.Pod) {
	if c.err != nil {
		return
	}

	executor := LitmusExecutor{
		ContainerName: c.exp.ExperimentDetails.TargetContainer,
		PodName:       pod.Name,
		Namespace:     c.exp.ExperimentDetails.AppNS,
		Clients:       c.exp.Clients,
	}

	c.err = c.injector.ResetChaosFn(executor, c.chaosParams)
}

func (c *litmusChaosOrchestrator) revertChaos(pods []corev1.Pod) {
	if c.err != nil {
		return
	}

	for _, pod := range pods {
		c.killChaos(pod)
	}
}

func (c *litmusChaosOrchestrator) abortChaos(pods []corev1.Pod) {
	if c.err != nil {
		return
	}

	log.Info("[Chaos] Revert started")
	c.revertChaos(pods)
	if c.err != nil {
		log.Errorf(
			"Error killing chaos after abortion: %v", c.err)
	}
	log.Info("[Chaos] Revert completed")
	os.Exit(1)
}

func (c *litmusChaosOrchestrator) observeAndReact(pods []corev1.Pod) {
	if c.err != nil {
		return
	}

observeLoop:
	for {
		c.startEndTimer()

		select {
		case err := <-c.errChannel:
			if err != nil {
				c.err = err
				break observeLoop
			}
		case <-c.signalChannel:
			c.abortChaos(pods)

		case <-c.endTime:
			log.Infof("[Chaos]: Tims is up for experiment",
				c.exp.ExperimentDetails.ExperimentName)
			break observeLoop
		}
	}

	c.revertChaos(pods)
}
