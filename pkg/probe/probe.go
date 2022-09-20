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
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

// RunProbes contains the steps to trigger the probes
// It contains steps to trigger all three probes: k8sprobe, httpprobe, cmdprobe
func RunProbes(chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {

	// get the probes details from the chaosengine
	probes, err := getProbesFromEngine(chaosDetails, clients)
	if err != nil {
		return err
	}

	switch strings.ToLower(phase) {
	//execute probes for the prechaos & duringchaos phase
	case "prechaos", "duringchaos":
		for _, probe := range probes {
			if err := execute(probe, chaosDetails, clients, resultDetails, phase); err != nil {
				return err
			}
		}
	default:
		// execute the probes for the postchaos phase
		// it first evaluate the onchaos and continuous modes then it evaluates the other modes
		// as onchaos and continuous probes are already completed
		var probeError []error
		for _, probe := range probes {
			// evaluate continuous and onchaos probes
			switch strings.ToLower(probe.Mode) {
			case "onchaos", "continuous":
				if err := execute(probe, chaosDetails, clients, resultDetails, phase); err != nil {
					probeError = append(probeError, err)
				}
			}
		}
		if len(probeError) != 0 {
			return errors.Errorf("probes failed, err: %v", probeError)
		}
		// executes the eot and edge modes
		for _, probe := range probes {
			if strings.ToLower(probe.Mode) != "onchaos" && strings.ToLower(probe.Mode) != "continuous" {
				if err := execute(probe, chaosDetails, clients, resultDetails, phase); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

//setProbeVerdict mark the verdict of the probe in the chaosresult as passed
// on the basis of phase(pre/post chaos)
func setProbeVerdict(resultDetails *types.ResultDetails, probe v1alpha1.ProbeAttributes, verdict v1alpha1.ProbeVerdict, description string) {
	for index, probes := range resultDetails.ProbeDetails {
		if probes.Name == probe.Name && probes.Type == probe.Type {
			if probes.Mode == "Edge" && probes.Status.Verdict == v1alpha1.ProbeVerdictFailed {
				return
			}
			resultDetails.ProbeDetails[index].Status = v1alpha1.ProbeStatus{
				Verdict:     verdict,
				Description: description,
			}
			break
		}
	}
}

//SetProbeVerdictAfterFailure mark the verdict of all the failed/unrun probes as failed
func SetProbeVerdictAfterFailure(result *v1alpha1.ChaosResult) {
	for index := range result.Status.ProbeStatuses {
		if result.Status.ProbeStatuses[index].Status.Verdict == v1alpha1.ProbeVerdictAwaited {
			result.Status.ProbeStatuses[index].Status.Verdict = v1alpha1.ProbeVerdictNA
			result.Status.ProbeStatuses[index].Status.Description = "either probe is not executed or not evaluated"
		}
	}
}

// getProbesFromEngine fetch the details of the probes from the chaosengines
func getProbesFromEngine(chaosDetails *types.ChaosDetails, clients clients.ClientSets) ([]v1alpha1.ProbeAttributes, error) {

	var Probes []v1alpha1.ProbeAttributes

	if err := retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			engine, err := clients.LitmusClient.ChaosEngines(chaosDetails.ChaosNamespace).Get(context.Background(), chaosDetails.EngineName, v1.GetOptions{})
			if err != nil {
				return fmt.Errorf("unable to Get the chaosengine, err: %v", err)
			}
			// get all the probes defined inside chaosengine for the corresponding experiment
			for _, experiment := range engine.Spec.Experiments {
				if experiment.Name == chaosDetails.ExperimentName {
					Probes = experiment.Spec.Probe
				}
			}
			return nil
		}); err != nil {
		return nil, err
	}

	return Probes, nil
}

// InitializeProbesInChaosResultDetails set the probe inside chaos result
// it fetch the probe details from the chaosengine and set into the chaosresult
func InitializeProbesInChaosResultDetails(chaosDetails *types.ChaosDetails, clients clients.ClientSets, chaosresult *types.ResultDetails) error {

	var probeDetails []types.ProbeDetails
	// get the probes from the chaosengine
	probes, err := getProbesFromEngine(chaosDetails, clients)
	if err != nil {
		return err
	}

	// set the probe details for k8s probe
	for _, probe := range probes {
		tempProbe := types.ProbeDetails{}
		tempProbe.Name = probe.Name
		tempProbe.Type = probe.Type
		tempProbe.Mode = probe.Mode
		tempProbe.RunCount = 0
		tempProbe.Status = v1alpha1.ProbeStatus{
			Verdict: "Awaited",
		}
		probeDetails = append(probeDetails, tempProbe)
	}

	chaosresult.ProbeDetails = probeDetails
	chaosresult.ProbeArtifacts = map[string]types.ProbeArtifact{}
	return nil
}

//getAndIncrementRunCount return the run count for the specified probe
func getAndIncrementRunCount(resultDetails *types.ResultDetails, probeName string) int {
	for index, probe := range resultDetails.ProbeDetails {
		if probeName == probe.Name {
			resultDetails.ProbeDetails[index].RunCount++
			return resultDetails.ProbeDetails[index].RunCount
		}
	}
	return 0
}

//getRunIDFromProbe return the run_id for the dedicated probe
// which will used in the continuous cmd probe, run_id is used as suffix in the external pod name
func getRunIDFromProbe(resultDetails *types.ResultDetails, probeName, probeType string) string {

	for _, probe := range resultDetails.ProbeDetails {
		if probe.Name == probeName && probe.Type == probeType {
			return probe.RunID
		}
	}
	return ""
}

//setRunIDForProbe set the run_id for the dedicated probe.
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
		description = getDescription(strings.ToLower(probe.Mode), phase)
	}

	setProbeVerdict(resultDetails, probe, probeVerdict, description)
	if !probe.RunProperties.StopOnFailure {
		return nil
	}
	return err
}

func getDescription(mode, phase string) string {
	switch mode {
	case "edge":
		return fmt.Sprintf("'%v' Probe didn't met the passing criteria", phase)
	default:
		return "Probe didn't met the passing criteria"
	}
}

//CheckForErrorInContinuousProbe check for the error in the continuous probes
func checkForErrorInContinuousProbe(resultDetails *types.ResultDetails, probeName string) error {

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
		return "", err
	}

	return out.String(), nil
}

// stopChaosEngine update the probe status and patch the chaosengine to stop state
func stopChaosEngine(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) error {
	// it will check for the error, It will detect the error if any error encountered in probe during chaos
	err = checkForErrorInContinuousProbe(chaosresult, probe.Name)
	// failing the probe, if the success condition doesn't met after the retry & timeout combinations
	markedVerdictInEnd(err, chaosresult, probe, "PostChaos")
	//patch chaosengine's state to stop
	engine, err := clients.LitmusClient.ChaosEngines(chaosDetails.ChaosNamespace).Get(context.Background(), chaosDetails.EngineName, v1.GetOptions{})
	if err != nil {
		return err
	}
	engine.Spec.EngineState = v1alpha1.EngineStateStop
	_, err = clients.LitmusClient.ChaosEngines(chaosDetails.ChaosNamespace).Update(context.Background(), engine, v1.UpdateOptions{})
	return err
}

// execute contains steps to execute & evaluate probes in different modes at different phases
func execute(probe v1alpha1.ProbeAttributes, chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, phase string) error {
	switch strings.ToLower(probe.Type) {
	case "k8sprobe":
		// it contains steps to prepare the k8s probe
		if err = prepareK8sProbe(probe, resultDetails, clients, phase, chaosDetails); err != nil {
			return errors.Errorf("probes failed, err: %v", err)
		}
	case "cmdprobe":
		// it contains steps to prepare cmd probe
		if err = prepareCmdProbe(probe, clients, chaosDetails, resultDetails, phase); err != nil {
			return errors.Errorf("probes failed, err: %v", err)
		}
	case "httpprobe":
		// it contains steps to prepare http probe
		if err = prepareHTTPProbe(probe, clients, chaosDetails, resultDetails, phase); err != nil {
			return errors.Errorf("probes failed, err: %v", err)
		}
	case "promprobe":
		// it contains steps to prepare prom probe
		if err = preparePromProbe(probe, clients, chaosDetails, resultDetails, phase); err != nil {
			return errors.Errorf("probes failed, err: %v", err)
		}
	default:
		return errors.Errorf("No supported probe type found, type: %v", probe.Type)
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
