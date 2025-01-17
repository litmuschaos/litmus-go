package probe

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/kyokomi/emoji"
	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

// RunProbes contains the steps to trigger the probes
// It contains steps to trigger all three probes: k8sprobe, httpprobe, cmdprobe
func RunProbes(ctx context.Context, chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "RunProbes")
	defer span.End()

	// get the probes details from the chaosengine
	probes, err := getProbesFromChaosEngine(chaosDetails, clients)
	if err != nil {
		span.SetStatus(codes.Error, "getProbesFromChaosEngine failed")
		span.RecordError(err)
		return err
	}

	switch strings.ToLower(phase) {
	//execute probes for the prechaos phase
	case "prechaos":
		for _, probe := range probes {
			switch strings.ToLower(probe.Mode) {
			case "sot", "edge", "continuous":
				if err := execute(probe, chaosDetails, clients, resultDetails, phase); err != nil {
					span.SetStatus(codes.Error, fmt.Sprintf("%s mode %s probe execute failed", probe.Mode, probe.Name))
					span.RecordError(err)
					return err
				}
			}
		}
	//execute probes for the duringchaos phase
	case "duringchaos":
		for _, probe := range probes {
			if strings.ToLower(probe.Mode) == "onchaos" {
				if err := execute(probe, chaosDetails, clients, resultDetails, phase); err != nil {
					span.SetStatus(codes.Error, fmt.Sprintf("%s mode %s probe execute failed", probe.Mode, probe.Name))
					span.RecordError(err)
					return err
				}
			}
		}
	default:
		// execute the probes for the postchaos phase
		// it first evaluate the onchaos and continuous modes then it evaluates the other modes
		// as onchaos and continuous probes are already completed
		var probeError []string
		// call cancel function from chaosDetails context
		chaosDetails.ProbeContext.CancelFunc()
		for _, probe := range probes {
			// evaluate continuous and onchaos probes
			switch strings.ToLower(probe.Mode) {
			case "onchaos", "continuous":
				if err := execute(probe, chaosDetails, clients, resultDetails, phase); err != nil {
					probeError = append(probeError, stacktrace.RootCause(err).Error())
				}
			}
		}
		if len(probeError) != 0 {
			errString := fmt.Sprintf("[%s]", strings.Join(probeError, ","))
			span.SetStatus(codes.Error, errString)
			err := cerrors.PreserveError{ErrString: errString}
			span.RecordError(err)
			return err
		}
		// executes the eot and edge modes
		for _, probe := range probes {
			switch strings.ToLower(probe.Mode) {
			case "eot", "edge":
				if err := execute(probe, chaosDetails, clients, resultDetails, phase); err != nil {
					span.SetStatus(codes.Error, fmt.Sprintf("%s mode %s probe execute failed", probe.Mode, probe.Name))
					span.RecordError(err)
					return err
				}
			}
		}
	}
	return nil
}

// setProbeVerdict mark the verdict of the probe in the chaosresult as passed
// on the basis of phase(pre/post chaos)
func setProbeVerdict(resultDetails *types.ResultDetails, probe v1alpha1.ProbeAttributes, verdict v1alpha1.ProbeVerdict, description, phase string) {
	for index, probes := range resultDetails.ProbeDetails {
		if probes.Name == probe.Name && probes.Type == probe.Type {
			// in edge mode, it will not update the verdict to pass in prechaos mode as probe verdict should be evaluated based on both the prechaos and postchaos results
			// in postchaos it will not override the verdict if verdict is already failed in prechaos
			if probes.Mode == "Edge" {
				if (phase == "PreChaos" && verdict != v1alpha1.ProbeVerdictFailed) || (phase == "PostChaos" && probes.Status.Verdict == v1alpha1.ProbeVerdictFailed) {
					return
				}
			}
			resultDetails.ProbeDetails[index].Status.Verdict = verdict
			if description != "" {
				resultDetails.ProbeDetails[index].Status.Description = description
			}
			break
		}
	}
}

// setProbeDescription sets the description to probe
func setProbeDescription(resultDetails *types.ResultDetails, probe v1alpha1.ProbeAttributes, description string) {
	for index, probes := range resultDetails.ProbeDetails {
		if probes.Name == probe.Name && probes.Type == probe.Type {
			resultDetails.ProbeDetails[index].Status.Description = description
			break
		}
	}
}

// SetProbeVerdictAfterFailure mark the verdict of all the failed/unrun probes as failed
func SetProbeVerdictAfterFailure(result *v1alpha1.ChaosResult) {
	for index := range result.Status.ProbeStatuses {
		if result.Status.ProbeStatuses[index].Status.Verdict == v1alpha1.ProbeVerdictAwaited {
			result.Status.ProbeStatuses[index].Status.Verdict = v1alpha1.ProbeVerdictNA
			result.Status.ProbeStatuses[index].Status.Description = "Either probe is not executed or not evaluated"
		}
	}
}

func getProbesFromChaosEngine(chaosDetails *types.ChaosDetails, clients clients.ClientSets) ([]v1alpha1.ProbeAttributes, error) {
	engine, err := types.GetChaosEngine(chaosDetails, clients)
	if err != nil {
		return nil, err
	}
	for _, exp := range engine.Spec.Experiments {
		if exp.Name == chaosDetails.ExperimentName {
			return exp.Spec.Probe, nil
		}
	}
	return nil, nil
}

// getAndIncrementRunCount return the run count for the specified probe
func getAndIncrementRunCount(resultDetails *types.ResultDetails, probeName string) int {
	for index, probe := range resultDetails.ProbeDetails {
		if probeName == probe.Name {
			resultDetails.ProbeDetails[index].RunCount++
			return resultDetails.ProbeDetails[index].RunCount
		}
	}
	return 0
}

// getRunIDFromProbe return the run_id for the dedicated probe
// which will used in the continuous cmd probe, run_id is used as suffix in the external pod name
func getRunIDFromProbe(resultDetails *types.ResultDetails, probeName, probeType string) string {

	for _, probe := range resultDetails.ProbeDetails {
		if probe.Name == probeName && probe.Type == probeType {
			return probe.RunID
		}
	}
	return ""
}

// setRunIDForProbe set the run_id for the dedicated probe.
// which will used in the continuous cmd probe, run_id is used as suffix in the external pod name
func setRunIDForProbe(resultDetails *types.ResultDetails, probeName, probeType, runid string) {
	for index, probe := range resultDetails.ProbeDetails {
		if probe.Name == probeName && probe.Type == probeType {
			resultDetails.ProbeDetails[index].RunID = runid
			break
		}
	}
}

// markedVerdictInEnd add the probe status in the chaosresult
func markedVerdictInEnd(err error, resultDetails *types.ResultDetails, probe v1alpha1.ProbeAttributes, phase string) error {
	probeVerdict := v1alpha1.ProbeVerdictPassed
	var description string
	if err != nil {
		probeVerdict = v1alpha1.ProbeVerdictFailed
	}

	switch probeVerdict {
	case v1alpha1.ProbeVerdictPassed:
		log.InfoWithValues("[Probe]: "+probe.Name+" probe has been Passed "+emoji.Sprint(":smile:"), logrus.Fields{
			"ProbeName":     probe.Name,
			"ProbeType":     probe.Type,
			"ProbeInstance": phase,
			"ProbeStatus":   probeVerdict,
		})
		// counting the passed probes count to generate the score and mark the verdict as passed
		// for edge, probe is marked as Passed if passed in both pre/post chaos checks
		switch strings.ToLower(probe.Mode) {
		case "edge":
			if phase == "PostChaos" && getProbeVerdict(resultDetails, probe.Name, probe.Type) != v1alpha1.ProbeVerdictFailed {
				resultDetails.PassedProbeCount++
			}
		default:
			resultDetails.PassedProbeCount++
		}
	default:
		log.ErrorWithValues("[Probe]: "+probe.Name+" probe has been Failed "+emoji.Sprint(":cry:"), logrus.Fields{
			"ProbeName":     probe.Name,
			"ProbeType":     probe.Type,
			"ProbeInstance": phase,
			"ProbeStatus":   probeVerdict,
		})
		description = getDescription(err)
	}

	setProbeVerdict(resultDetails, probe, probeVerdict, description, phase)

	if err != nil {
		switch probe.RunProperties.StopOnFailure {
		case true:
			// adding signal to communicate that experiment is stopped because of error in probe
			if probeDetails := getProbeByName(probe.Name, resultDetails.ProbeDetails); probeDetails != nil {
				probeDetails.Stopped = true
			}
			return err
		default:
			if probeDetails := getProbeByName(probe.Name, resultDetails.ProbeDetails); probeDetails != nil {
				probeDetails.IsProbeFailedWithError = err
			}
			return nil
		}
	}
	return nil
}

// getProbeByName returns the probe details of a probe given its name
func getProbeByName(name string, probeDetails []*types.ProbeDetails) *types.ProbeDetails {
	for _, p := range probeDetails {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func getProbeTimeouts(name string, probeDetails []*types.ProbeDetails) types.ProbeTimeouts {
	probe := getProbeByName(name, probeDetails)
	if probe != nil {
		return probe.Timeouts
	}
	return types.ProbeTimeouts{}
}

func getDescription(err error) string {
	rootCause := stacktrace.RootCause(err)
	if error, ok := rootCause.(cerrors.Error); ok {
		return error.Reason
	}
	return rootCause.Error()
}

// CheckForErrorInContinuousProbe check for the error in the continuous probes
func checkForErrorInContinuousProbe(resultDetails *types.ResultDetails, probeName string, delay int, timeout int) error {

	probe := getProbeByName(probeName, resultDetails.ProbeDetails)
	startTime := time.Now()
	timeoutSignal := time.After(time.Duration(timeout) * time.Second)

loop:
	for {
		select {
		case <-timeoutSignal:
			return cerrors.Error{
				ErrorCode: cerrors.FailureTypeProbeTimeout,
				Target:    fmt.Sprintf("{probe: %s, timeout: %ds}", probeName, timeout),
				Reason:    "Probe is failed due to timeout",
			}
		default:
			if probe.HasProbeCompleted {
				break loop
			}
			log.Infof("[Probe]: Waiting for %s probe to finish or timeout (Elapsed time: %v s)", probeName, time.Since(startTime).Seconds())
			time.Sleep(time.Duration(delay) * time.Second)
		}
	}
	for index, probe := range resultDetails.ProbeDetails {
		if probe.Name == probeName {
			return resultDetails.ProbeDetails[index].IsProbeFailedWithError
		}
	}
	return nil
}

// ParseCommand parse the templated command and replace the templated value by actual value
// if command doesn't have template, it will return the same command
func parseCommand(templatedCommand string, resultDetails *types.ResultDetails) (string, error) {

	register := resultDetails.ProbeArtifacts

	t := template.Must(template.New("t1").Parse(templatedCommand))

	// store the parsed output in the buffer
	var out bytes.Buffer
	if err := t.Execute(&out, register); err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("failed to parse the templated command, %s", err.Error())}
	}

	return out.String(), nil
}

// stopChaosEngine update the probe status and patch the chaosengine to stop state
func stopChaosEngine(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) error {
	// it will check for the error, It will detect the error if any error encountered in probe during chaos
	if err = checkForErrorInContinuousProbe(chaosresult, probe.Name, chaosDetails.Timeout, chaosDetails.Delay); err != nil && cerrors.GetErrorType(err) != cerrors.FailureTypeProbeTimeout {
		return err
	}

	// failing the probe, if the success condition doesn't met after the retry & timeout combinations
	markedVerdictInEnd(err, chaosresult, probe, "PostChaos")
	//patch chaosengine's state to stop
	engine, err := clients.LitmusClient.ChaosEngines(chaosDetails.ChaosNamespace).Get(context.Background(), chaosDetails.EngineName, v1.GetOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("failed to get chaosengine, %s", err.Error())}
	}
	engine.Spec.EngineState = v1alpha1.EngineStateStop
	_, err = clients.LitmusClient.ChaosEngines(chaosDetails.ChaosNamespace).Update(context.Background(), engine, v1.UpdateOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("failed to patch the chaosengine to `stop` state, %v", err.Error())}
	}

	return nil
}

// execute contains steps to execute & evaluate probes in different modes at different phases
func execute(probe v1alpha1.ProbeAttributes, chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, phase string) error {
	switch strings.ToLower(probe.Type) {
	case "k8sprobe":
		// it contains steps to prepare the k8s probe
		if err = prepareK8sProbe(probe, resultDetails, clients, phase, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "probes failed")
		}
	case "cmdprobe":
		// it contains steps to prepare cmd probe
		if err = prepareCmdProbe(probe, clients, chaosDetails, resultDetails, phase); err != nil {
			return stacktrace.Propagate(err, "probes failed")
		}
	case "httpprobe":
		// it contains steps to prepare http probe
		if err = prepareHTTPProbe(probe, clients, chaosDetails, resultDetails, phase); err != nil {
			return stacktrace.Propagate(err, "probes failed")
		}
	case "promprobe":
		// it contains steps to prepare prom probe
		if err = preparePromProbe(probe, clients, chaosDetails, resultDetails, phase); err != nil {
			return stacktrace.Propagate(err, "probes failed")
		}
	default:
		return stacktrace.Propagate(err, "%v probe type not supported", probe.Type)
	}
	return nil
}

func getProbeVerdict(resultDetails *types.ResultDetails, name, probeType string) v1alpha1.ProbeVerdict {
	for _, probe := range resultDetails.ProbeDetails {
		if probe.Name == name && probe.Type == probeType {
			return probe.Status.Verdict
		}
	}
	return v1alpha1.ProbeVerdictNA
}

func addProbePhase(err error, phase string) error {
	rootCause := stacktrace.RootCause(err)
	if error, ok := rootCause.(cerrors.Error); ok {
		error.Phase = phase
		err = error
	}
	return err
}

func getAttempts(attempt, retries int) int {
	if attempt == 0 && retries == 0 {
		return 1
	}
	if attempt == 0 {
		return retries
	}
	return attempt
}

func IsProbeFailed(reason string) bool {
	if strings.Contains(reason, string(cerrors.FailureTypeK8sProbe)) || strings.Contains(reason, string(cerrors.FailureTypePromProbe)) ||
		strings.Contains(reason, string(cerrors.FailureTypeCmdProbe)) || strings.Contains(reason, string(cerrors.FailureTypeHttpProbe)) {
		return true
	}
	return false
}

func checkProbeTimeoutError(name string, code cerrors.ErrorType, probeErr error) error {
	log.Infof("name: %s, err: %v", name, probeErr)
	if cerrors.GetErrorType(probeErr) == cerrors.ErrorTypeTimeout {
		return cerrors.Error{
			ErrorCode: code,
			Target:    fmt.Sprintf("{name: %s}", name),
			Reason:    "probe failed due to timeout",
		}
	}
	return probeErr
}
