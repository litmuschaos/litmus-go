package result

import (
	"strconv"
	"time"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//ChaosResult Create and Update the chaos result
func ChaosResult(chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, state string) error {

	// It will list all the chaos-result with matching label
	// it will retries until it got chaos result list or met the timeout(3 mins)
	// Note: We have added labels inside chaos result and looking for matching labels to list the chaos-result
	var resultList *v1alpha1.ChaosResultList
	err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			result, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).List(metav1.ListOptions{LabelSelector: "name=" + resultDetails.Name})
			if err != nil {
				return errors.Errorf("Unable to list the chaosresult, err: %v", err)
			}
			resultList = result
			return nil
		})

	if err != nil {
		return err
	}

	// if there is no chaos-result with given label, it will create a new chaos-result
	if len(resultList.Items) == 0 {
		return InitializeChaosResult(chaosDetails, clients, resultDetails)
	}

	for _, result := range resultList.Items {

		// the chaos-result is already present with matching labels
		// it will patch the new parameters in the same chaos-result
		if state == "SOT" {
			return PatchChaosResult(&result, clients, chaosDetails, resultDetails)
		}

		// it will patch the chaos-result in the end of experiment
		resultDetails.Phase = "Completed"
		return PatchChaosResult(&result, clients, chaosDetails, resultDetails)
	}
	return nil
}

//InitializeChaosResult create the chaos result
func InitializeChaosResult(chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails) error {

	probeStatus := GetProbeStatus(resultDetails)
	chaosResult := &v1alpha1.ChaosResult{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resultDetails.Name,
			Namespace: chaosDetails.ChaosNamespace,
			Labels: map[string]string{
				"name": resultDetails.Name,
			},
		},
		Spec: v1alpha1.ChaosResultSpec{
			EngineName:     chaosDetails.EngineName,
			ExperimentName: chaosDetails.ExperimentName,
			InstanceID:     chaosDetails.InstanceID,
		},
		Status: v1alpha1.ChaosResultStatus{
			ExperimentStatus: v1alpha1.TestStatus{
				Phase:   resultDetails.Phase,
				Verdict: resultDetails.Verdict,
			},
			ProbeStatus: probeStatus,
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
		chaosResult, err = clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(resultDetails.Name, metav1.GetOptions{})
		if err != nil {
			return errors.Errorf("Unable to get the chaosresult, err: %v", err)
		}
		// adding the labels to the chaosresult
		chaosResult.ObjectMeta.Labels = map[string]string{
			"name": resultDetails.Name,
		}
		// updating the chaosresult with new values
		err = PatchChaosResult(chaosResult, clients, chaosDetails, resultDetails)
		if err != nil {
			return err
		}

	}

	return nil
}

//GetProbeStatus fetch status of all probes
func GetProbeStatus(resultDetails *types.ResultDetails) []v1alpha1.ProbeStatus {

	probeStatus := []v1alpha1.ProbeStatus{}
	for _, probe := range resultDetails.ProbeDetails {
		probes := v1alpha1.ProbeStatus{}
		probes.Name = probe.Name
		probes.Type = probe.Type
		probes.Status = probe.Status
		probeStatus = append(probeStatus, probes)
	}
	return probeStatus
}

//PatchChaosResult Update the chaos result
func PatchChaosResult(result *v1alpha1.ChaosResult, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	result.Status.ExperimentStatus.Phase = resultDetails.Phase
	result.Status.ExperimentStatus.Verdict = resultDetails.Verdict
	result.Spec.InstanceID = chaosDetails.InstanceID
	result.Status.ExperimentStatus.FailStep = resultDetails.FailStep
	result.Status.ProbeStatus = GetProbeStatus(resultDetails)
	if resultDetails.Phase == "Completed" {
		if resultDetails.Verdict == "Pass" && len(resultDetails.ProbeDetails) != 0 {

			result.Status.ExperimentStatus.ProbeSuccessPercentage = "100"

		} else if resultDetails.Verdict == "Fail" && len(resultDetails.ProbeDetails) != 0 {

			result.Status.ExperimentStatus.ProbeSuccessPercentage = strconv.Itoa((resultDetails.PassedProbeCount * 100) / len(resultDetails.ProbeDetails))
		}

	} else if len(resultDetails.ProbeDetails) != 0 {
		result.Status.ExperimentStatus.ProbeSuccessPercentage = "Awaited"
	}

	// It will update the existing chaos-result CR with new values
	// it will retries until it will able to update successfully or met the timeout(3 mins)
	err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			_, err := clients.LitmusClient.ChaosResults(result.Namespace).Update(result)
			if err != nil {
				return errors.Errorf("Unable to update the chaosresult, err: %v", err)
			}
			return nil
		})

	return err
}

// SetResultUID sets the ResultUID into the ResultDetails structure
func SetResultUID(resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	result, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(resultDetails.Name, metav1.GetOptions{})

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
	msg := chaosDetails.ExperimentName + " experiment has been " + resultDetails.Verdict + "ed"
	types.SetResultEventAttributes(eventsDetails, types.Summary, msg, "Warning", resultDetails)
	events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")

	// add the summary event in chaos engine
	if chaosDetails.EngineName != "" {
		types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

}
