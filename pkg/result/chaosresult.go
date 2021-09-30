package result

import (
	"bytes"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//ChaosResult Create and Update the chaos result
func ChaosResult(chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, state string) error {
	experimentLabel := map[string]string{}

	// It try to get the chaosresult, if available
	// it will retries until it got chaos result or met the timeout(3 mins)
	var result *v1alpha1.ChaosResult
	isResultAvailable := false
	if err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			resultObj, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(resultDetails.Name, v1.GetOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Errorf("unable to get %v chaosresult in %v namespace, err: %v", resultDetails.Name, chaosDetails.ChaosNamespace, err)
			} else if err == nil {
				result = resultObj
				isResultAvailable = true
			}
			return nil
		}); err != nil {
		return err
	}

	// as the chaos pod won't be available for stopped phase
	// skipping the derivation of labels from chaos pod, if phase is stopped
	if chaosDetails.EngineName != "" && resultDetails.Phase != "Stopped" {
		// Getting chaos pod label and passing it in chaos result
		chaosPod, err := clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).Get(chaosDetails.ChaosPodName, v1.GetOptions{})
		if err != nil {
			return errors.Errorf("failed to find chaos pod with name: %v, err: %v", chaosDetails.ChaosPodName, err)
		}
		experimentLabel = chaosPod.Labels
	}
	experimentLabel["chaosUID"] = string(chaosDetails.ChaosUID)

	// if there is no chaos-result with given name, it will create a new chaos-result
	if !isResultAvailable {
		return InitializeChaosResult(chaosDetails, clients, resultDetails, experimentLabel)
	}

	// the chaos-result is already present with matching labels
	// it will patch the new parameters in the same chaos-result
	if state == "SOT" {
		updateHistory(result)
		return PatchChaosResult(result, clients, chaosDetails, resultDetails, experimentLabel)
	}

	// it will patch the chaos-result in the end of experiment
	resultDetails.Phase = v1alpha1.ResultPhaseCompleted
	return PatchChaosResult(result, clients, chaosDetails, resultDetails, experimentLabel)
}

//InitializeChaosResult create the chaos result
func InitializeChaosResult(chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosResultLabel map[string]string) error {

	_, probeStatus := GetProbeStatus(resultDetails)
	chaosResult := &v1alpha1.ChaosResult{
		ObjectMeta: v1.ObjectMeta{
			Name:      resultDetails.Name,
			Namespace: chaosDetails.ChaosNamespace,
			Labels:    chaosResultLabel,
		},
		Spec: v1alpha1.ChaosResultSpec{
			EngineName:     chaosDetails.EngineName,
			ExperimentName: chaosDetails.ExperimentName,
			InstanceID:     chaosDetails.InstanceID,
		},
		Status: v1alpha1.ChaosResultStatus{
			ExperimentStatus: v1alpha1.TestStatus{
				Phase:                  resultDetails.Phase,
				Verdict:                resultDetails.Verdict,
				ProbeSuccessPercentage: "Awaited",
			},
			ProbeStatus: probeStatus,
			History: v1alpha1.HistoryDetails{
				PassedRuns:  0,
				FailedRuns:  0,
				StoppedRuns: 0,
				Targets:     []v1alpha1.TargetDetails{},
			},
		},
	}

	// It will create a new chaos-result CR
	_, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Create(chaosResult)

	// if the chaos result is already present, it will patch the new parameters with the existing chaos result CR
	// Note: We have added labels inside chaos result and looking for matching labels to list the chaos-result
	// these labels were not present inside earlier releases so giving a retry/update if someone have a exiting result CR
	// in his cluster, which was created earlier with older release/version of litmus.
	// it will override the params and add the labels to it so that it will work as desired.
	if k8serrors.IsAlreadyExists(err) {
		chaosResult, err = clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(resultDetails.Name, v1.GetOptions{})
		if err != nil {
			return errors.Errorf("Unable to find the chaosresult with name %v in %v namespace, err: %v", resultDetails.Name, chaosDetails.ChaosNamespace, err)
		}

		// updating the chaosresult with new values
		if err = PatchChaosResult(chaosResult, clients, chaosDetails, resultDetails, chaosResultLabel); err != nil {
			return err
		}
	}
	return nil
}

//GetProbeStatus fetch status of all probes
func GetProbeStatus(resultDetails *types.ResultDetails) (bool, []v1alpha1.ProbeStatus) {
	isAllProbePassed := true

	probeStatus := []v1alpha1.ProbeStatus{}
	for _, probe := range resultDetails.ProbeDetails {
		probes := v1alpha1.ProbeStatus{}
		probes.Name = probe.Name
		probes.Type = probe.Type
		probes.Status = probe.Status
		probeStatus = append(probeStatus, probes)
		if probe.Phase == "Failed" {
			isAllProbePassed = false
		}
	}
	return isAllProbePassed, probeStatus
}

//PatchChaosResult Update the chaos result
func PatchChaosResult(result *v1alpha1.ChaosResult, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, chaosResultLabel map[string]string) error {

	annotations, err := GetChaosStatus(resultDetails, chaosDetails, clients)
	if err != nil {
		return err
	}

	var isAllProbePassed bool
	result.Status.ExperimentStatus.Phase = resultDetails.Phase
	result.Spec.InstanceID = chaosDetails.InstanceID
	result.Status.ExperimentStatus.FailStep = resultDetails.FailStep
	// for existing chaos result resource it will patch the label
	result.ObjectMeta.Labels = chaosResultLabel
	result.ObjectMeta.Annotations = annotations
	result.Status.History.Targets = chaosDetails.Targets
	isAllProbePassed, result.Status.ProbeStatus = GetProbeStatus(resultDetails)
	result.Status.ExperimentStatus.Verdict = resultDetails.Verdict

	switch strings.ToLower(string(resultDetails.Phase)) {
	case "completed":
		if !isAllProbePassed {
			resultDetails.Verdict = "Fail"
			result.Status.ExperimentStatus.Verdict = "Fail"
		}
		switch strings.ToLower(string(resultDetails.Verdict)) {
		case "pass":
			result.Status.ExperimentStatus.ProbeSuccessPercentage = "100"
			result.Status.History.PassedRuns++
		case "fail":
			result.Status.History.FailedRuns++
			probe.SetProbeVerdictAfterFailure(resultDetails)
			if len(resultDetails.ProbeDetails) != 0 {
				result.Status.ExperimentStatus.ProbeSuccessPercentage = strconv.Itoa((resultDetails.PassedProbeCount * 100) / len(resultDetails.ProbeDetails))
			} else {
				result.Status.ExperimentStatus.ProbeSuccessPercentage = "0"
			}
		case "stopped":
			result.Status.History.StoppedRuns++
			probe.SetProbeVerdictAfterFailure(resultDetails)
			if len(resultDetails.ProbeDetails) != 0 {
				result.Status.ExperimentStatus.ProbeSuccessPercentage = strconv.Itoa((resultDetails.PassedProbeCount * 100) / len(resultDetails.ProbeDetails))
			} else {
				result.Status.ExperimentStatus.ProbeSuccessPercentage = "0"
			}
		}
	default:
		result.Status.ExperimentStatus.ProbeSuccessPercentage = "Awaited"
	}

	// It will update the existing chaos-result CR with new values
	// it will retries until it will able to update successfully or met the timeout(3 mins)
	return retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			_, err := clients.LitmusClient.ChaosResults(result.Namespace).Update(result)
			if err != nil {
				return errors.Errorf("Unable to update the chaosresult, err: %v", err)
			}
			return nil
		})
}

// SetResultUID sets the ResultUID into the ResultDetails structure
func SetResultUID(resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	result, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(resultDetails.Name, v1.GetOptions{})
	if err != nil {
		return err
	}

	resultDetails.ResultUID = result.UID
	return nil
}

//RecordAfterFailure update the chaosresult and create the summary events
func RecordAfterFailure(chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, failStep string, clients clients.ClientSets, eventsDetails *types.EventDetails) {

	// update the chaos result
	types.SetResultAfterCompletion(resultDetails, "Fail", "Completed", failStep)
	ChaosResult(chaosDetails, clients, resultDetails, "EOT")

	// add the summary event in chaos result
	msg := "experiment: " + chaosDetails.ExperimentName + ", Result: " + string(resultDetails.Verdict)
	types.SetResultEventAttributes(eventsDetails, types.FailVerdict, msg, "Warning", resultDetails)
	events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")

	// add the summary event in chaos engine
	if chaosDetails.EngineName != "" {
		types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

}

// updateHistory initialise the history for the older results
func updateHistory(result *v1alpha1.ChaosResult) {
	if reflect.DeepEqual(result.Status.History, v1alpha1.HistoryDetails{}) {
		history := v1alpha1.HistoryDetails{
			PassedRuns:  0,
			FailedRuns:  0,
			StoppedRuns: 0,
			Targets:     []v1alpha1.TargetDetails{},
		}
		result.Status.History = history
	}
}

// AnnotateChaosResult annotate the chaosResult for the chaos status
// using kubectl cli to annotate the chaosresult as it will automatically handle the race condition in case of multiple helpers
func AnnotateChaosResult(resultName, namespace, status, kind, name string) error {
	command := exec.Command("kubectl", "annotate", "chaosresult", resultName, "-n", namespace, kind+"/"+name+"="+status, "--overwrite")
	var out, stderr bytes.Buffer
	command.Stdout = &out
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		log.Infof("Error String: %v", stderr.String())
		return errors.Errorf("unable to annotate the %v chaosresult, err: %v", resultName, err)
	}
	return nil
}

// GetChaosStatus get the chaos status based on annotations in chaosresult
func GetChaosStatus(resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, clients clients.ClientSets) (map[string]string, error) {

	result, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(resultDetails.Name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	annotations := result.ObjectMeta.Annotations
	targetList := []v1alpha1.TargetDetails{}
	for k, v := range annotations {
		switch strings.ToLower(v) {
		case "injected", "reverted", "targeted":
			kind := strings.TrimSpace(strings.Split(k, "/")[0])
			name := strings.TrimSpace(strings.Split(k, "/")[1])
			target := v1alpha1.TargetDetails{
				Name:        name,
				Kind:        kind,
				ChaosStatus: v,
			}
			targetList = append(targetList, target)
			delete(annotations, k)
		}
	}

	chaosDetails.Targets = append(chaosDetails.Targets, targetList...)
	return annotations, nil
}
