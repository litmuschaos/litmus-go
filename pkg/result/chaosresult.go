package result

import (
	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/environment"
	"github.com/litmuschaos/litmus-go/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/klog"
)

//ChaosResult Create/Update the chaos result
func ChaosResult(eventsDetails *types.EventDetails, clients environment.ClientSets, resultDetails *types.ResultDetails, state string) error {

	result, _ := clients.LitmusClient.ChaosResults(eventsDetails.ChaosNamespace).Get(resultDetails.Name, metav1.GetOptions{})

	if state == "SOT" {

		if result.Name == resultDetails.Name {
			err := PatchChaosResult(result, clients, eventsDetails, resultDetails)
			if err != nil {
				return err
			}
		} else {
			err := InitializeChaosResult(eventsDetails, clients, resultDetails)
			if err != nil {
				return err
			}
		}
	} else {
		resultDetails.Phase = "Completed"
		err := PatchChaosResult(result, clients, eventsDetails, resultDetails)
		if err != nil {
			return err
		}
	}

	return nil
}

//InitializeChaosResult create the chaos result
func InitializeChaosResult(eventsDetails *types.EventDetails, clients environment.ClientSets, resultDetails *types.ResultDetails) error {

	chaosResult := &v1alpha1.ChaosResult{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resultDetails.Name,
			Namespace: eventsDetails.ChaosNamespace,
		},
		Spec: v1alpha1.ChaosResultSpec{
			EngineName:     eventsDetails.EngineName,
			ExperimentName: eventsDetails.ExperimentName,
			InstanceID:     eventsDetails.InstanceID,
		},
		Status: v1alpha1.ChaosResultStatus{
			ExperimentStatus: v1alpha1.TestStatus{
				Phase:   resultDetails.Phase,
				Verdict: resultDetails.Verdict,
			},
		},
	}
	_, err := clients.LitmusClient.ChaosResults(eventsDetails.ChaosNamespace).Create(chaosResult)
	return err
}

//PatchChaosResult Update the chaos result
func PatchChaosResult(result *v1alpha1.ChaosResult, clients environment.ClientSets, eventsDetails *types.EventDetails, resultDetails *types.ResultDetails) error {

	result.Status.ExperimentStatus.Phase = resultDetails.Phase
	result.Status.ExperimentStatus.Verdict = resultDetails.Verdict
	result.Spec.InstanceID = eventsDetails.InstanceID
	result.Status.ExperimentStatus.FailStep = resultDetails.FailStep

	_, err := clients.LitmusClient.ChaosResults(result.Namespace).Update(result)

	return err
}
