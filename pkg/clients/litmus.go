package clients

import (
	"context"
	"fmt"
	"time"

	clientTypes "k8s.io/apimachinery/pkg/types"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (clients *ClientSets) GetChaosEngine(chaosDetails *types.ChaosDetails) (*v1alpha1.ChaosEngine, error) {
	var (
		engine *v1alpha1.ChaosEngine
		err    error
	)

	if err := retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			engine, err = clients.LitmusClient.ChaosEngines(chaosDetails.ChaosNamespace).Get(context.Background(), chaosDetails.EngineName, v1.GetOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: err.Error(), Target: fmt.Sprintf("{engineName: %s, engineNs: %s}", chaosDetails.EngineName, chaosDetails.ChaosNamespace)}
			}
			return nil
		}); err != nil {
		return nil, err
	}

	return engine, nil
}

func (clients *ClientSets) UpdateChaosEngine(chaosDetails *types.ChaosDetails, engine *v1alpha1.ChaosEngine) error {
	return retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			_, err := clients.LitmusClient.ChaosEngines(chaosDetails.ChaosNamespace).Update(context.Background(), engine, v1.UpdateOptions{})
			return err
		})
}

func (clients *ClientSets) GetChaosResult(chaosDetails *types.ChaosDetails) (*v1alpha1.ChaosResult, error) {
	var result v1alpha1.ChaosResult

	if err := retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			resultList, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).List(context.Background(),
				v1.ListOptions{LabelSelector: fmt.Sprintf("chaosUID=%s", chaosDetails.ChaosUID)})
			if err != nil {
				return err
			}
			if len(resultList.Items) == 0 {
				return nil
			}
			result = resultList.Items[0]
			return nil
		}); err != nil {
		return nil, err
	}

	return &result, nil
}

func (clients *ClientSets) CreateChaosResult(chaosDetails *types.ChaosDetails, result *v1alpha1.ChaosResult) (bool, error) {
	var exists bool

	err := retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			_, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Create(context.Background(), result, v1.CreateOptions{})
			if err != nil {
				if k8serrors.IsAlreadyExists(err) {
					exists = true
					return nil
				}
				return err
			}
			return nil
		})

	return exists, err
}

func (clients *ClientSets) UpdateChaosResult(chaosDetails *types.ChaosDetails, result *v1alpha1.ChaosResult) error {

	err := retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			_, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Update(context.Background(), result, v1.UpdateOptions{})
			if err != nil {
				return err
			}
			return nil
		})

	return err
}

func (clients *ClientSets) PatchChaosResult(chaosDetails *types.ChaosDetails, resultName string, mergePatch []byte) error {

	err := retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			_, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Patch(context.TODO(), resultName, clientTypes.MergePatchType, mergePatch, v1.PatchOptions{})
			if err != nil {
				return err
			}
			return nil
		})

	return err
}
