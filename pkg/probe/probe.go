package probe

import (
	"fmt"

	"github.com/kyokomi/emoji"
	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

// RunProbes contains the steps to trigger the probes
// It contains steps to trigger all three probes: k8sprobe, httpprobe, cmdprobe
func RunProbes(chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {

	// get the probes details from the chaosengine
	k8sProbes, cmdProbes, httpProbes, err := GetProbesFromEngine(chaosDetails, clients)
	if err != nil {
		return err
	}

	// it contains steps to prepare the k8s probe
	err = PrepareK8sProbe(k8sProbes, resultDetails, clients, phase, eventsDetails, chaosDetails)
	if err != nil {
		return err
	}

	// it contains steps to prepare cmd probe
	err = PrepareCmdProbe(cmdProbes, clients, chaosDetails, resultDetails, phase, eventsDetails)
	if err != nil {
		return err
	}

	// it contains steps to prepare http probe
	err = PrepareHTTPProbe(httpProbes, clients, chaosDetails, resultDetails, phase, eventsDetails)
	if err != nil {
		return err
	}

	return nil
}

//SetProbeVerdict mark the verdict of the probe in the chaosresult as passed
// on the basis of phase(pre/post chaos)
func SetProbeVerdict(resultDetails *types.ResultDetails, verdict, probeName, probeType, mode, phase string) {

	for index, probe := range resultDetails.ProbeDetails {
		if probe.Name == probeName && probe.Type == probeType {

			if phase == "PreChaos" && (mode == "SOT" || mode == "Edge") {
				resultDetails.ProbeDetails[index].Status["PreChaos"] = verdict + emoji.Sprint(" :thumbsup:")
			} else if phase == "PostChaos" && (mode == "EOT" || mode == "Edge") {
				resultDetails.ProbeDetails[index].Status["PostChaos"] = verdict + emoji.Sprint(" :thumbsup:")
			} else if phase == "PostChaos" && mode == "Continuous" {
				resultDetails.ProbeDetails[index].Status["Continuous"] = verdict + emoji.Sprint(" :thumbsup:")
			}
		}
	}
}

//SetProbeVerdictAfterFailure mark the verdict of all the failed/unrun probes as failed
func SetProbeVerdictAfterFailure(resultDetails *types.ResultDetails) {
	for index := range resultDetails.ProbeDetails {
		if resultDetails.ProbeDetails[index].Status["PreChaos"] == "Awaited" {
			resultDetails.ProbeDetails[index].Status["PreChaos"] = "Better Luck Next Time" + emoji.Sprint(" :thumbsdown:")
		}
		if resultDetails.ProbeDetails[index].Status["PostChaos"] == "Awaited" {
			resultDetails.ProbeDetails[index].Status["PostChaos"] = "Better Luck Next Time" + emoji.Sprint(" :thumbsdown:")
		}
		if resultDetails.ProbeDetails[index].Status["Continuous"] == "Awaited" {
			resultDetails.ProbeDetails[index].Status["Continuous"] = "Better Luck Next Time" + emoji.Sprint(" :thumbsdown:")
		}
	}
}

// GetProbesFromEngine fetch the details of the probes from the chaosengines
func GetProbesFromEngine(chaosDetails *types.ChaosDetails, clients clients.ClientSets) ([]v1alpha1.K8sProbeAttributes, []v1alpha1.CmdProbeAttributes, []v1alpha1.HTTPProbeAttributes, error) {

	engine, err := clients.LitmusClient.ChaosEngines(chaosDetails.ChaosNamespace).Get(chaosDetails.EngineName, v1.GetOptions{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Unable to Get the chaosengine due to %v", err)
	}

	// define all the probes
	var k8sProbes []v1alpha1.K8sProbeAttributes
	var httpProbes []v1alpha1.HTTPProbeAttributes
	var cmdProbes []v1alpha1.CmdProbeAttributes

	// get all the probes defined inside chaosengine for the corresponding experiment
	experimentSpec := engine.Spec.Experiments
	for _, experiment := range experimentSpec {

		if experiment.Name == chaosDetails.ExperimentName {

			k8sProbes = experiment.Spec.K8sProbe
			httpProbes = experiment.Spec.HTTPProbe
			cmdProbes = experiment.Spec.CmdProbe
		}
	}

	return k8sProbes, cmdProbes, httpProbes, nil
}

// InitializeProbesInChaosResultDetails set the probe inside chaos result
// it fetch the probe details from the chaosengine and set into the chaosresult
func InitializeProbesInChaosResultDetails(chaosDetails *types.ChaosDetails, clients clients.ClientSets, chaosresult *types.ResultDetails) error {

	probeDetails := []types.ProbeDetails{}
	probeDetail := types.ProbeDetails{}
	// get the probes from the chaosengine
	k8sProbes, cmdProbes, httpProbes, err := GetProbesFromEngine(chaosDetails, clients)
	if err != nil {
		return err
	}

	// set the probe details for k8s probe
	for _, probe := range k8sProbes {
		probeDetail.Name = probe.Name
		probeDetail.Type = "K8sProbe"
		SetProbeIntialStatus(&probeDetail, probe.Mode)
		probeDetails = append(probeDetails, probeDetail)
	}

	// set the probe details for http probe
	for _, probe := range httpProbes {
		probeDetail.Name = probe.Name
		probeDetail.Type = "HTTPProbe"
		SetProbeIntialStatus(&probeDetail, probe.Mode)
		probeDetails = append(probeDetails, probeDetail)
	}

	// set the probe details for cmd probe
	for _, probe := range cmdProbes {
		probeDetail.Name = probe.Name
		probeDetail.Type = "CmdProbe"
		SetProbeIntialStatus(&probeDetail, probe.Mode)
		probeDetails = append(probeDetails, probeDetail)
	}

	chaosresult.ProbeDetails = probeDetails

	return nil
}

//SetProbeIntialStatus sets the initial status inside chaosresult
func SetProbeIntialStatus(probeDetails *types.ProbeDetails, mode string) {
	if mode == "Edge" {
		probeDetails.Status = map[string]string{
			"PreChaos":  "Awaited",
			"PostChaos": "Awaited",
		}
	} else if mode == "SOT" {
		probeDetails.Status = map[string]string{
			"PreChaos": "Awaited",
		}
	} else if mode == "EOT" {
		probeDetails.Status = map[string]string{
			"PostChaos": "Awaited",
		}
	} else {
		probeDetails.Status = map[string]string{
			"Continuous": "Awaited",
		}
	}
}

//GetRunIDFromProbe return the run_id for the dedicated probe
// which will used in the continuous cmd probe, run_id is used as suffix in the external pod name
func GetRunIDFromProbe(resultDetails *types.ResultDetails, probeName, probeType string) string {

	for _, probe := range resultDetails.ProbeDetails {
		if probe.Name == probeName && probe.Type == probeType {
			return probe.RunID
		}
	}
	return ""
}

//SetRunIDForProbe set the run_id for the dedicated probe.
// which will used in the continuous cmd probe, run_id is used as suffix in the external pod name
func SetRunIDForProbe(resultDetails *types.ResultDetails, probeName, probeType, runid string) {

	for index, probe := range resultDetails.ProbeDetails {
		if probe.Name == probeName && probe.Type == probeType {
			resultDetails.ProbeDetails[index].RunID = runid
			break
		}
	}
}

// MarkedVerdictInEnd add the probe status in the chaosresult
func MarkedVerdictInEnd(err error, resultDetails *types.ResultDetails, probeName, mode, probeType, phase string) error {
	// failing the probe, if the success condition doesn't met after the retry & timeout combinations
	if err != nil {
		log.ErrorWithValues("[Probe]: "+probeName+" probe has been Failed "+emoji.Sprint(":cry:"), logrus.Fields{
			"ProbeName":     probeName,
			"ProbeType":     probeType,
			"ProbeInstance": phase,
			"ProbeStatus":   "Failed",
		})
		SetProbeVerdictAfterFailure(resultDetails)
		return err
	}
	// counting the passed probes count to generate the score and mark the verdict as passed
	// for edge, probe is marked as Passed if passed in both pre/post chaos checks
	if !((mode == "Edge" || mode == "Continuous") && phase == "PreChaos") {
		resultDetails.PassedProbeCount++
	}
	log.InfoWithValues("[Probe]: "+probeName+" probe has been Passed "+emoji.Sprint(":smile:"), logrus.Fields{
		"ProbeName":     probeName,
		"ProbeType":     probeType,
		"ProbeInstance": phase,
		"ProbeStatus":   "Passed",
	})
	SetProbeVerdict(resultDetails, "Passed", probeName, probeType, mode, phase)
	return nil
}

//CheckForErrorInContinuousProbe check for the error in the continuous probes
func CheckForErrorInContinuousProbe(resultDetails *types.ResultDetails, probeName string) error {

	for index, probe := range resultDetails.ProbeDetails {
		if probe.Name == probeName {
			err = resultDetails.ProbeDetails[index].IsProbeFailedWithError
			return err
		}
	}

	return nil
}
