package kafka

import (
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LivenessCleanup will generate kafka liveness deployment on the basic of given condition
func LivenessCleanup(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	var err error

	err = DeleteLivenessPod(experimentsDetails, clients)
	if err != nil {
		return err
	}

	return nil
}

// DeleteLivenessPod will check the deletion of liveness pod
func DeleteLivenessPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	if err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaoslibDetail.AppNS).Delete("kafka-liveness-"+experimentsDetails.RunID, &metav1.DeleteOptions{}); err != nil {
		return errors.Errorf("Fail to delete liveness deployment, due to %v", err)
	}

	err := retry.
		Times(uint(experimentsDetails.ChaoslibDetail.Timeout / experimentsDetails.ChaoslibDetail.Delay)).
		Wait(time.Duration(experimentsDetails.ChaoslibDetail.Delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaoslibDetail.AppNS).List(metav1.ListOptions{LabelSelector: "name=kafka-liveness"})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Liveness pod is not deleted yet, err: %v", err)
			}
			return nil
		})
	return err
}
