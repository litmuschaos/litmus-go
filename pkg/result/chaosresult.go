package result

import (
	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//ChaosResult Create and Update the chaos result
func ChaosResult(chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, state string) error {

	result, _ := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(resultDetails.Name, metav1.GetOptions{})

	if state == "SOT" {

		if result.Name == resultDetails.Name {
			err := PatchChaosResult(result, clients, chaosDetails, resultDetails)
			if err != nil {
				return err
			}
		} else {
			err := InitializeChaosResult(chaosDetails, clients, resultDetails)
			if err != nil {
				return err
			}
		}
	} else {
		resultDetails.Phase = "Completed"
		err := PatchChaosResult(result, clients, chaosDetails, resultDetails)
		if err != nil {
			return err
		}
	}

	return nil
}

//InitializeChaosResult create the chaos result
func InitializeChaosResult(chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails) error {

	chaosResult := &v1alpha1.ChaosResult{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resultDetails.Name,
			Namespace: chaosDetails.ChaosNamespace,
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
		},
	}
	_, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Create(chaosResult)
	return err
}

//PatchChaosResult Update the chaos result
func PatchChaosResult(result *v1alpha1.ChaosResult, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	result.Status.ExperimentStatus.Phase = resultDetails.Phase
	result.Status.ExperimentStatus.Verdict = resultDetails.Verdict
	result.Spec.InstanceID = chaosDetails.InstanceID
	result.Status.ExperimentStatus.FailStep = resultDetails.FailStep

	_, err := clients.LitmusClient.ChaosResults(result.Namespace).Update(result)

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
