package experiment

import (
	"fmt"

	litmusLib "github.com/litmuschaos/litmus-go/chaoslib/litmus/pod-io-error-retval/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"

	// we borrow pod-memory hog's types package since it has almost everything we require
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-memory-hog/types"

	// we also borrow the environment package from pod-memory-hog to read the relevant environment variables
	experimentEnvironment "github.com/litmuschaos/litmus-go/pkg/generic/pod-memory-hog/environment"
)

type SafeExperiment struct {
	exp litmusLib.ExperimentOrchestrationDetails
	err error
}

func SafeExperimentInstance(clients clients.ClientSets) SafeExperiment {
	return SafeExperiment{
		exp: litmusLib.ExperimentOrchestrationDetails{
			ExperimentDetails: &experimentTypes.ExperimentDetails{},
			Clients:           clients,
			ResultDetails:     &types.ResultDetails{},
			EventDetails:      &types.EventDetails{},
			ChaosDetails:      &types.ChaosDetails{},
		},
		err: nil,
	}
}

func (e *SafeExperiment) InitializeExperimentFromEnvironment() {
	if e.err != nil {
		return
	}

	experimentEnvironment.GetENV(e.exp.ExperimentDetails)
	experimentEnvironment.InitialiseChaosVariables(
		e.exp.ChaosDetails, e.exp.ExperimentDetails)

	types.SetResultAttributes(
		e.exp.ResultDetails, *e.exp.ChaosDetails)
}

func (e *SafeExperiment) InitializeProbes() {
	if e.err != nil || e.exp.ExperimentDetails.EngineName == "" {
		return
	}

	e.err = probe.InitializeProbesInChaosResultDetails(
		e.exp.ChaosDetails, e.exp.Clients, e.exp.ResultDetails)

	if e.err != nil {
		log.Errorf("Unable to initialize probes, err: %v", e.err)
	}
}

func (e *SafeExperiment) ObtainChaosResult() {
	if e.err != nil {
		return
	}

	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)",
		e.exp.ExperimentDetails.ExperimentName)

	e.err = result.ChaosResult(
		e.exp.ChaosDetails, e.exp.Clients, e.exp.ResultDetails, "SOT")

	if e.err == nil {
		return
	}

	log.Errorf("Unable to create chaos result, err :%v", e.err)
	result.RecordAfterFailure(
		e.exp.ChaosDetails,
		e.exp.ResultDetails,
		"Updating the chaos result of pod-memory-hog experiment (SOT)",
		e.exp.Clients,
		e.exp.EventDetails,
	)
}

func (e *SafeExperiment) SetResultUid() {
	if e.err != nil {
		return
	}

	result.SetResultUID(
		e.exp.ResultDetails,
		e.exp.Clients,
		e.exp.ChaosDetails,
	)
}

func (e *SafeExperiment) GenerateEvent(reason string, message string, eventType string) {
	types.SetResultEventAttributes(
		e.exp.EventDetails, reason, message, eventType, e.exp.ResultDetails)

	events.GenerateEvents(
		e.exp.EventDetails, e.exp.Clients, e.exp.ChaosDetails, "ChaosResult")
}

func (e *SafeExperiment) GenerateExperimentAwaitedEvent() {
	if e.err != nil {
		return
	}

	experimentName := e.exp.ExperimentDetails.ExperimentName
	e.GenerateEvent(
		types.AwaitedVerdict,
		fmt.Sprintf(
			"experiment: %s, Result: Awaited",
			experimentName,
		),
		"Normal",
	)
}

func (e *SafeExperiment) LogApplicationInformation() {
	if e.err != nil {
		return
	}

	expDetails := e.exp.ExperimentDetails
	log.InfoWithValues("The application information is as follows",
		logrus.Fields{
			"Namespace":          expDetails.AppNS,
			"Label":              expDetails.AppLabel,
			"Chaos Duration":     expDetails.ChaosDuration,
			"Ramp Time":          expDetails.RampTime,
			"Memory Consumption": expDetails.MemoryConsumption,
		},
	)
}

func (e *SafeExperiment) LaunchAbortWatcher() {
	if e.err != nil {
		return
	}

	// Calling AbortWatcher go routine, it will continuously
	// watch for the abort signal and generate the required
	// events and result
	go common.AbortWatcherWithoutExit(
		e.exp.ExperimentDetails.ExperimentName,
		e.exp.Clients,
		e.exp.ResultDetails,
		e.exp.ChaosDetails,
		e.exp.EventDetails,
	)
}

func (e *SafeExperiment) RecordFailure(errFmt string, failStep string) {
	log.Errorf(errFmt, e.err)
	result.RecordAfterFailure(
		e.exp.ChaosDetails,
		e.exp.ResultDetails,
		failStep,
		e.exp.Clients,
		e.exp.EventDetails,
	)
}

// phase = (pre chaos) | (post chaos)
func (e *SafeExperiment) VerifyAppUnderTestRunning(phase string) {
	if e.err != nil {
		return
	}

	log.Infof("[Status]: Verify that the AUT (Application Under Test) is running %s", phase)
	e.err = status.AUTStatusCheck(
		e.exp.ExperimentDetails.AppNS,
		e.exp.ExperimentDetails.AppLabel,
		e.exp.ExperimentDetails.TargetContainer,
		e.exp.ExperimentDetails.Timeout,
		e.exp.ExperimentDetails.Delay,
		e.exp.Clients,
		e.exp.ChaosDetails,
	)

	if e.err == nil {
		return
	}

	e.RecordFailure(
		"Application status check failed, err: %v",
		fmt.Sprintf("Verify that the AUT (Application Under Test) is running %s", phase),
	)
}

// phase = PreChaos | PostChaos
func (e *SafeExperiment) RunProbesCheck(phase string) {
	if e.err != nil || e.exp.ExperimentDetails.EngineName == "" {
		return
	}

	reason := fmt.Sprintf("%sCheck", phase)

	if len(e.exp.ResultDetails.ProbeDetails) == 0 {
		e.GenerateEvent(reason, "AUT: Running", "Normal")
		return
	}

	e.err = probe.RunProbes(
		e.exp.ChaosDetails, e.exp.Clients, e.exp.ResultDetails, phase, e.exp.EventDetails)

	if e.err == nil {
		e.GenerateEvent(reason, "AUT: Running, Probes: Successful", "Normal")
	} else {
		e.RecordFailure("Probe failed. err: %v", "Failed while running probes")
		e.GenerateEvent(reason, "AUT: Running, Probes: Unsuccessful", "Warning")
	}
}

func (e *SafeExperiment) RunExperiment(chaosInjector litmusLib.ChaosInjector) {
	if e.err != nil {
		return
	}

	errFmt, failStep := "", ""

	switch e.exp.ExperimentDetails.ChaosLib {
	case "litmus":
		e.err = litmusLib.OrchestrateExperiment(e.exp, chaosInjector)
		errFmt = "[Error]: pod memory hog failed, err: %v"
		failStep = "failed in chaos injection phase"
	default:
		e.err = fmt.Errorf("[Invalid]: Please provide correct lib")
		errFmt = "%v"
		failStep = "No match found for specified lib."
	}

	if e.err != nil {
		e.RecordFailure(errFmt, failStep)
	} else {
		log.Info("Chaos injected sucessfully.")
		e.exp.ResultDetails.Verdict = "Pass"
	}
}

func (e *SafeExperiment) RecordEndOfExperiment() {
	if e.err != nil {
		return
	}

	log.Infof("[The End]: Updating the chaos result of %v experiment (EOT)",
		e.exp.ExperimentDetails.ExperimentName)
	e.err = result.ChaosResult(
		e.exp.ChaosDetails, e.exp.Clients, e.exp.ResultDetails, "EOT")

	if e.err != nil {
		log.Errorf("Unable to Update the Chaos Result, err: %v", e.err)
	}
}

func (e *SafeExperiment) GenerateVerdictEvent() {
	if e.err != nil {
		return
	}

	msg := fmt.Sprintf(
		"experiment: %s, Result: %s",
		e.exp.ExperimentDetails.ExperimentName,
		e.exp.ResultDetails.Verdict,
	)

	if e.exp.ResultDetails.Verdict != "Pass" {
		e.GenerateEvent(types.FailVerdict, msg, "Warning")
	} else {
		e.GenerateEvent(types.PassVerdict, msg, "Normal")
	}

	if e.exp.ExperimentDetails.ExperimentName != "" {
		e.GenerateEvent(
			types.Summary,
			fmt.Sprintf(
				"%s experiments has been %sed",
				e.exp.ExperimentDetails.ExperimentName,
				e.exp.ResultDetails.Verdict,
			),
			"Normal",
		)
	}
}

func PodChaosExperiment(clients clients.ClientSets, chaosInjector litmusLib.ChaosInjector) {
	experiment := SafeExperimentInstance(clients)

	experiment.InitializeExperimentFromEnvironment()
	experiment.InitializeProbes()
	experiment.ObtainChaosResult()

	experiment.SetResultUid()
	experiment.GenerateExperimentAwaitedEvent()
	experiment.LogApplicationInformation()

	experiment.LaunchAbortWatcher()

	experiment.VerifyAppUnderTestRunning("(pre chaos)")
	experiment.RunProbesCheck("PreChaos")

	experiment.RunExperiment(chaosInjector)

	experiment.VerifyAppUnderTestRunning("(post chaos)")
	experiment.RunProbesCheck("PostChaos")

	experiment.RecordEndOfExperiment()
	experiment.GenerateVerdictEvent()
}
