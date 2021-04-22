// Package pod-io-error-retval/lib is a library for orchestrating
// io-error chaos experiments based on the debugfs fail function feature.
package lib

import (
	"fmt"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-memory-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/types"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Command is a type for representing executable commands on a shell
type Command string

// Executor is an interface to represent exec() providers.
type Executor interface {
	Execute(command Command) error
}

// Shell is an abstraction to lift errors from Executor(s). It wraps
// error values to provide safe successive command execution.
type Shell struct {
	executor Executor
	err      error
}

// Run safely invokes the Executor
func (shell *Shell) Run(command Command) {
	if shell.err != nil {
		return
	}

	shell.err = shell.executor.Execute(command)
}

// Script represents a sequence of commands
type Script []Command

// RunOn runs the script on the given shell
func (script Script) RunOn(shell Shell) error {
	for _, command := range script {
		shell.Run(command)
	}

	return shell.err
}

// FailFunctionArgs is a data struct to account for the various
// parameters required for writing to /sys/kernel/debug/fail_function
type FailFunctionArgs struct {
	FuncName    string // name of the function to be injected
	RetVal      int    // value to be returned
	Probability int    // fail probability [0-100]
	Interval    int    // interval between two failures, if interval > 1,
	// we recommend setting fail probability to 100
}

// FailFunctionScript returns the sequence of commands required to inject
// fail_function failure.
func FailFunctionScript(args FailFunctionArgs) Script {
	pathPrefix := "/sys/kernel/debug/fail_function"

	return []Command{
		Command(fmt.Sprintf("echo %s > %s/inject", args.FuncName, pathPrefix)),
		Command(fmt.Sprintf("echo %d > %s/%s/retval", args.RetVal, pathPrefix, args.FuncName)),
		Command(fmt.Sprintf("echo N  > %s/task-filter", pathPrefix)),
		Command(fmt.Sprintf("echo %d > %s/probability", args.Probability, pathPrefix)),
		Command(fmt.Sprintf("echo %d > %s/interval", args.Interval, pathPrefix)),
		Command(fmt.Sprintf("echo -1 > %s/times", pathPrefix)),
		Command(fmt.Sprintf("echo 0  > %s/space", pathPrefix)),
		Command(fmt.Sprintf("echo 1  > %s/verbose", pathPrefix)),
	}
}

// ResetFailFunctionScript return the sequence of commands for reseting failure injection.
func ResetFailFunctionScript() Script {
	return []Command{Command("echo > /sys/kernel/debug/fail_function/inject")}
}

// LitmusExecutor implements the Executor interface to executing commands on pods
type LitmusExecutor struct {
	ContainerName string
	PodName       string
	Namespace     string
	Clients       clients.ClientSets
}

// Execute executes commands on the current container in the current pod
func (executor LitmusExecutor) Execute(command Command) error {
	log.Infof("Executing command: $%s", command)

	execCommandDetails := litmusexec.PodDetails{}
	cmd := []string{"/bin/sh", "-c", string(command)}

	litmusexec.SetExecCommandAttributes(
		&execCommandDetails,
		executor.PodName,
		executor.ContainerName,
		executor.Namespace,
	)

	_, err := litmusexec.Exec(
		&execCommandDetails, executor.Clients, cmd)

	return err
}

// InjectFailFunction injects a fail function failure with the given executor and the given
// fail_function arguments. It writes errors if any, or nil in an error channel passed to
// this function.
func InjectFailFunction(executor Executor, failFunctionArgs FailFunctionArgs, errChannel chan error) {
	errChannel <- FailFunctionScript(failFunctionArgs).RunOn(Shell{executor: executor, err: nil})
}

// ExperimentOrchestrationDetails bundles together all details connected to the orchestration
// of and experiment.
type ExperimentOrchestrationDetails struct {
	ExperimentDetails *experimentTypes.ExperimentDetails
	Clients           clients.ClientSets
	ResultDetails     *types.ResultDetails
	EventDetails      *types.EventDetails
	ChaosDetails      *types.ChaosDetails
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

	targetPodList corev1.PodList
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

	exp.targetPodList, exp.err = common.GetPodList(
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
	for _, pod := range exp.targetPodList.Items {
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

	experimentDetails.TargetContainer, exp.err =
		TargetContainer(exp.experiment, exp.targetPodList.Items[0].Name)

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

// OrchestrateExperiment orchestrates a new chaos experiment with the given experiment details
// and the ChaosInjector for the chaos injection mechanism.
func OrchestrateExperiment(exp ExperimentOrchestrationDetails, chaosInjector ChaosInjector) error {
	safeExperimentOrchestrator := safeExperiment{experiment: exp}

	safeExperimentOrchestrator.verifyAppLabelOrTargetPodSpecified()
	safeExperimentOrchestrator.obtainTargetPods()
	safeExperimentOrchestrator.logTargetPodNames()
	safeExperimentOrchestrator.obtainTargetContainer()
	safeExperimentOrchestrator.injectChaos(chaosInjector)

	return safeExperimentOrchestrator.err
}
