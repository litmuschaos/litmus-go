package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LivenessCleanup deletes the kafka liveness pod
func LivenessCleanup(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	if err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaoslibDetail.AppNS).Delete(context.Background(), "kafka-liveness-"+experimentsDetails.RunID, metav1.DeleteOptions{}); err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Reason: fmt.Sprintf("fail to delete liveness deployment, %s", err.Error())}
	}

	return retry.
		Times(uint(experimentsDetails.ChaoslibDetail.Timeout / experimentsDetails.ChaoslibDetail.Delay)).
		Wait(time.Duration(experimentsDetails.ChaoslibDetail.Delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaoslibDetail.AppNS).List(context.Background(), metav1.ListOptions{LabelSelector: "name=kafka-liveness-" + experimentsDetails.RunID})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Reason: fmt.Sprintf("liveness pod is not deleted yet, %s", err.Error())}
			} else if len(podSpec.Items) != 0 {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Reason: "liveness pod is not deleted yet"}
			}
			return nil
		})
}
