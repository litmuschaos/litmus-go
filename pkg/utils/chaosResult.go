package utils

import (
	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

// CreateChaosResultStart Generate the chaos result CR to reflect SOT (Start of Test)
func CreateChaosResultStart(experimentsDetails *ExperimentDetails, clients ClientSets) error {

	result, _ := clients.LitmusClient.ChaosResults(experimentsDetails.ChaosNamespace).Get(experimentsDetails.EngineName+"-"+experimentsDetails.ExperimentName, metav1.GetOptions{})

	if result.Name == experimentsDetails.EngineName+"-"+experimentsDetails.ExperimentName {
		Uerr := UpdateChaosResultStart(result, clients, experimentsDetails)
		if Uerr != nil {
			return Uerr
		}
	} else {
		Cerr := CreateChaosResult(experimentsDetails, clients)
		if Cerr != nil {
			return Cerr
		}
	}

	return nil
}

// CreateChaosResult Generate the chaos result CR to reflect SOT (Start of Test)
func CreateChaosResult(experimentsDetails *ExperimentDetails, clients ClientSets) error {

	chaosResult := &v1alpha1.ChaosResult{
		ObjectMeta: metav1.ObjectMeta{
			Name:      experimentsDetails.EngineName + "-" + experimentsDetails.ExperimentName,
			Namespace: experimentsDetails.ChaosNamespace,
		},
		Spec: v1alpha1.ChaosResultSpec{
			EngineName:     experimentsDetails.EngineName,
			ExperimentName: experimentsDetails.ExperimentName,
			InstanceID:     experimentsDetails.InstanceID,
		},
		Status: v1alpha1.ChaosResultStatus{
			ExperimentStatus: v1alpha1.TestStatus{
				Phase:   "Running",
				Verdict: "Awaited",
			},
		},
	}
	_, err := clients.LitmusClient.ChaosResults(experimentsDetails.ChaosNamespace).Create(chaosResult)
	return err
}

// UpdateChaosResultMiddle Update the chaos result (Middle of Test)
func UpdateChaosResultMiddle(experimentsDetails *ExperimentDetails, clients ClientSets, flag string, failStep string) {

	klog.V(0).Infof("[The End]: Updating the chaos result of %v experiment (EOT)",experimentsDetails.ExperimentName)
	err:= UpdateChaosResultEnd(experimentsDetails,clients,flag,failStep)
	if err != nil {
		klog.V(0).Infof("Unable to Update the Chaos Result due to %v\n", err)
	} 

}

// UpdateChaosResultEnd Update the chaos result (End of Test)
func UpdateChaosResultEnd(experimentsDetails *ExperimentDetails, clients ClientSets, flag string, failStep string) error {

	result, err := clients.LitmusClient.ChaosResults(experimentsDetails.ChaosNamespace).Get(experimentsDetails.EngineName+"-"+experimentsDetails.ExperimentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	result.Status.ExperimentStatus.Phase = "Completed"
	result.Status.ExperimentStatus.Verdict = flag
	result.Status.ExperimentStatus.FailStep = failStep
	_, err = clients.LitmusClient.ChaosResults(experimentsDetails.ChaosNamespace).Update(result)
	if err != nil {
		return err
	}

	return nil
}

// UpdateChaosResultStart Update the chaos result (End of Test)
func UpdateChaosResultStart(result *v1alpha1.ChaosResult, clients ClientSets, experimentsDetails *ExperimentDetails) error {

	result.Status.ExperimentStatus.Phase = "Running"
	result.Status.ExperimentStatus.Verdict = "Awaited"
	result.Spec.InstanceID = experimentsDetails.InstanceID

	_, err := clients.LitmusClient.ChaosResults(result.Namespace).Update(result)

	return err
}
