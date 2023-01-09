package result

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChaosResult Create and Update the chaos result
func ChaosResult(chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, state string) error {
	experimentLabel := map[string]string{}

	// It tries to get the chaosresult, if available
	// it will retry until it got chaos result or met the timeout(3 mins)
	isResultAvailable := false
	if err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			_, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(context.Background(), resultDetails.Name, v1.GetOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosResultCRUD, Target: fmt.Sprintf("{name: %s, namespace: %s}", resultDetails.Name, chaosDetails.ChaosNamespace), Reason: err.Error()}
			} else if err == nil {
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
		chaosPod, err := clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).Get(context.Background(), chaosDetails.ChaosPodName, v1.GetOptions{})
		if err != nil {
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{name: %s, namespace: %s}", chaosDetails.ChaosPodName, chaosDetails.ChaosNamespace), Reason: fmt.Sprintf("failed to get experiment pod :%s", err.Error())}
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
		return PatchChaosResult(clients, chaosDetails, resultDetails, experimentLabel)
	}

	// it will patch the chaos-result in the end of experiment
	resultDetails.Phase = v1alpha1.ResultPhaseCompleted
	return PatchChaosResult(clients, chaosDetails, resultDetails, experimentLabel)
}

// InitializeChaosResult create the chaos result
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
			ProbeStatuses: probeStatus,
			History: &v1alpha1.HistoryDetails{
				PassedRuns:  0,
				FailedRuns:  0,
				StoppedRuns: 0,
				Targets:     []v1alpha1.TargetDetails{},
			},
		},
	}

	// It will create a new chaos-result CR
	_, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Create(context.Background(), chaosResult, v1.CreateOptions{})

	// if the chaos result is already present, it will patch the new parameters with the existing chaos result CR
	// Note: We have added labels inside chaos result and looking for matching labels to list the chaos-result
	// these labels were not present inside earlier releases so giving a retry/update if someone has an exiting result CR
	// in his cluster, which was created earlier with older release/version of litmus.
	// it will override the params and add the labels to it so that it will work as desired.
	if k8serrors.IsAlreadyExists(err) {
		_, err = clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(context.Background(), resultDetails.Name, v1.GetOptions{})
		if err != nil {
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosResultCRUD, Target: fmt.Sprintf("{name: %s, namespace: %s}", resultDetails.Name, chaosDetails.ChaosNamespace), Reason: err.Error()}
		}

		// updating the chaosresult with new values
		if err = PatchChaosResult(clients, chaosDetails, resultDetails, chaosResultLabel); err != nil {
			return stacktrace.Propagate(err, "could not update chaos result")
		}
	}
	return nil
}

// GetProbeStatus fetch status of all probes
func GetProbeStatus(resultDetails *types.ResultDetails) (bool, []v1alpha1.ProbeStatuses) {
	isAllProbePassed := true

	probeStatus := []v1alpha1.ProbeStatuses{}
	for _, probe := range resultDetails.ProbeDetails {
		probes := v1alpha1.ProbeStatuses{}
		probes.Name = probe.Name
		probes.Type = probe.Type
		probes.Mode = probe.Mode
		probes.Status = probe.Status
		probeStatus = append(probeStatus, probes)
		if probe.Status.Verdict == v1alpha1.ProbeVerdictFailed {
			isAllProbePassed = false
		}
	}
	return isAllProbePassed, probeStatus
}

func getFailStep(probeDetails []types.ProbeDetails, phase string) (string, string) {
	var (
		errList   []string
		errCode   cerrors.ErrorType
		rootCause string
	)
	for _, probe := range probeDetails {
		if probe.IsProbeFailedWithError != nil {
			rootCause, errCode = cerrors.GetRootCauseAndErrorCode(probe.IsProbeFailedWithError, phase)
			errList = append(errList, rootCause)
		}
	}

	if len(errList) != 0 {
		if len(errList) == 1 {
			return errList[0], string(errCode)
		}
		return fmt.Sprintf("[%v]", strings.Join(errList, ",")), string(cerrors.ErrorTypeGeneric)
	}
	return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: "probe didn't met the passing criteria"}.Error(), string(cerrors.ErrorTypeGeneric)
}

func updateResultAttributes(clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, chaosResultLabel map[string]string) (*v1alpha1.ChaosResult, error) {
	result, err := GetChaosStatus(resultDetails, chaosDetails, clients)
	if err != nil {
		return nil, stacktrace.Propagate(err, "could not get chaos status")
	}

	updateHistory(result)
	var isAllProbePassed bool
	result.Status.ExperimentStatus.Phase = resultDetails.Phase
	result.Spec.InstanceID = chaosDetails.InstanceID
	if resultDetails.FailureOutput != nil || resultDetails.Phase == v1alpha1.ResultPhaseRunning {
		result.Status.ExperimentStatus.FailureOutput = resultDetails.FailureOutput
	}

	// for existing chaos result resource it will patch the label
	result.ObjectMeta.Labels = chaosResultLabel
	result.Status.History.Targets = chaosDetails.Targets
	isAllProbePassed, result.Status.ProbeStatuses = GetProbeStatus(resultDetails)
	result.Status.ExperimentStatus.Verdict = resultDetails.Verdict

	switch strings.ToLower(string(resultDetails.Phase)) {
	case "completed":
		if !isAllProbePassed {
			resultDetails.Verdict = "Fail"
			result.Status.ExperimentStatus.Verdict = "Fail"
			if result.Status.ExperimentStatus.FailureOutput == nil || result.Status.ExperimentStatus.FailureOutput.FailedStep == "" {
				failStep, errCode := getFailStep(resultDetails.ProbeDetails, string(chaosDetails.Phase))
				result.Status.ExperimentStatus.FailureOutput = &v1alpha1.FailureOutput{
					FailedStep: failStep,
					ErrorCode:  errCode,
				}
			}
		}
		switch strings.ToLower(string(resultDetails.Verdict)) {
		case "pass":
			result.Status.ExperimentStatus.ProbeSuccessPercentage = "100"
			result.Status.History.PassedRuns++
		case "fail":
			result.Status.History.FailedRuns++
			probe.SetProbeVerdictAfterFailure(result)
			if len(resultDetails.ProbeDetails) != 0 {
				result.Status.ExperimentStatus.ProbeSuccessPercentage = strconv.Itoa((resultDetails.PassedProbeCount * 100) / len(resultDetails.ProbeDetails))
			} else {
				result.Status.ExperimentStatus.ProbeSuccessPercentage = "0"
			}
		case "stopped":
			result.Status.History.StoppedRuns++
			probe.SetProbeVerdictAfterFailure(result)
			if len(resultDetails.ProbeDetails) != 0 {
				result.Status.ExperimentStatus.ProbeSuccessPercentage = strconv.Itoa((resultDetails.PassedProbeCount * 100) / len(resultDetails.ProbeDetails))
			} else {
				result.Status.ExperimentStatus.ProbeSuccessPercentage = "0"
			}
		}
	default:
		result.Status.ExperimentStatus.ProbeSuccessPercentage = "Awaited"
	}
	return result, nil
}

// PatchChaosResult Update the chaos result
func PatchChaosResult(clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, chaosResultLabel map[string]string) error {

	result, err := updateResultAttributes(clients, chaosDetails, resultDetails, chaosResultLabel)
	if err != nil {
		return stacktrace.Propagate(err, "could not update chaosresult attributes")
	}

	// It will update the existing chaos-result CR with new values
	// it will retry until it will be able to update successfully or met the timeout(3 mins)
	return retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			_, updateErr := clients.LitmusClient.ChaosResults(result.Namespace).Update(context.Background(), result, v1.UpdateOptions{})
			if updateErr != nil {
				if k8serrors.IsConflict(updateErr) {
					result, err = updateResultAttributes(clients, chaosDetails, resultDetails, chaosResultLabel)
					if err != nil {
						return stacktrace.Propagate(err, "could not update chaosresult attributes")
					}
				}
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosResultCRUD, Phase: getExperimentPhaseFromResultPhase(resultDetails.Phase), Target: fmt.Sprintf("{name: %s, namespace: %s}", resultDetails.Name, chaosDetails.ChaosNamespace), Reason: updateErr.Error()}
			}
			return nil
		})
}

// SetResultUID sets the ResultUID into the ResultDetails structure
func SetResultUID(resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	result, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(context.Background(), resultDetails.Name, v1.GetOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosResultCRUD, Target: fmt.Sprintf("{name: %s, namespace: %s}", resultDetails.Name, chaosDetails.ChaosNamespace), Reason: err.Error()}
	}

	resultDetails.ResultUID = result.UID
	return nil
}

// RecordAfterFailure update the chaosresult and create the summary events
func RecordAfterFailure(chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, err error, clients clients.ClientSets, eventsDetails *types.EventDetails) {
	failStep, errorCode := cerrors.GetRootCauseAndErrorCode(err, string(chaosDetails.Phase))

	// update the chaos result
	types.SetResultAfterCompletion(resultDetails, "Fail", "Completed", failStep, errorCode)
	if err := ChaosResult(chaosDetails, clients, resultDetails, "EOT"); err != nil {
		log.Errorf("failed to update chaosresult, err: %v", err)
	}

	// add the summary event in chaos result
	msg := "experiment: " + chaosDetails.ExperimentName + ", Result: " + string(resultDetails.Verdict)
	types.SetResultEventAttributes(eventsDetails, types.FailVerdict, msg, "Warning", resultDetails)
	if err := events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult"); err != nil {
		log.Errorf("failed to create %v event inside chaosresult", types.FailVerdict)
	}

	// add the summary event in chaos engine
	if chaosDetails.EngineName != "" {
		types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
		if err := events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine"); err != nil {
			log.Errorf("failed to create %v event inside chaosengine", types.Summary)
		}
	}

}

// updateHistory initialise the history for the older results
func updateHistory(result *v1alpha1.ChaosResult) {
	if result.Status.History == nil {
		result.Status.History = &v1alpha1.HistoryDetails{
			PassedRuns:  0,
			FailedRuns:  0,
			StoppedRuns: 0,
			Targets:     []v1alpha1.TargetDetails{},
		}
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
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosResultCRUD, Target: fmt.Sprintf("{name: %s, namespace: %s}", resultName, namespace), Reason: out.String()}
	}
	return nil
}

// GetChaosStatus get the chaos status based on annotations in chaosresult
func GetChaosStatus(resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, clients clients.ClientSets) (*v1alpha1.ChaosResult, error) {

	result, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(context.Background(), resultDetails.Name, v1.GetOptions{})
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosResultCRUD, Target: fmt.Sprintf("{name: %s, namespace: %s}", resultDetails.Name, chaosDetails.ChaosNamespace), Reason: err.Error()}
	}
	annotations := result.ObjectMeta.Annotations
	targetList := chaosDetails.Targets
	for k, v := range annotations {
		switch strings.ToLower(v) {
		case "injected", "reverted", "targeted":
			kind := strings.TrimSpace(strings.Split(k, "/")[0])
			name := strings.TrimSpace(strings.Split(k, "/")[1])
			if !updateTargets(name, v, targetList) {
				targetList = append(targetList, v1alpha1.TargetDetails{
					Name:        name,
					Kind:        kind,
					ChaosStatus: v,
				})
			}
			delete(annotations, k)
		}
	}

	chaosDetails.Targets = targetList
	result.Annotations = annotations
	return result, nil
}

func UpdateFailedStepFromHelper(resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, client clients.ClientSets, err error) error {
	rootCause, errCode := cerrors.GetRootCauseAndErrorCode(err, string(chaosDetails.Phase))
	return retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			chaosResult, err := client.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(context.Background(), resultDetails.Name, v1.GetOptions{})
			if err != nil {
				return err
			}
			if chaosResult.Status.ExperimentStatus.FailureOutput != nil {
				chaosResult.Status.ExperimentStatus.FailureOutput.FailedStep = appendFailStep(chaosResult.Status.ExperimentStatus.FailureOutput.FailedStep, rootCause)
			} else {
				chaosResult.Status.ExperimentStatus.FailureOutput = &v1alpha1.FailureOutput{
					FailedStep: rootCause,
					ErrorCode:  string(errCode),
				}
			}
			_, err = client.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Update(context.Background(), chaosResult, v1.UpdateOptions{})
			return err
		})
}

func appendFailStep(failStep string, rootCause string) string {
	failStep = strings.TrimPrefix(failStep, "[")
	failStep = strings.TrimSuffix(failStep, "]")
	return fmt.Sprintf("[%s,%s]", failStep, rootCause)
}

// updates the chaos status of targets which is already present inside history.targets
func updateTargets(name, status string, data []v1alpha1.TargetDetails) bool {
	for i := range data {
		if data[i].Name == name {
			data[i].ChaosStatus = status
			return true
		}
	}
	return false
}

func getExperimentPhaseFromResultPhase(phase v1alpha1.ResultPhase) string {
	switch phase {
	case v1alpha1.ResultPhaseRunning:
		return "PreChaos"
	case v1alpha1.ResultPhaseCompleted:
		return "PostChaos"
	case v1alpha1.ResultPhaseStopped:
		return "Abort"
	}
	return ""
}
