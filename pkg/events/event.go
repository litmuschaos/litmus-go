package events

import (
	"time"

	"github.com/litmuschaos/litmus-go/pkg/environment"
	"github.com/litmuschaos/litmus-go/pkg/types"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//CreateEvents create the events
func CreateEvents(experimentsDetails *types.ExperimentDetails, clients environment.ClientSets, eventsDetails *types.EventDetails) error {

	events := &apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      eventsDetails.Reason + string(experimentsDetails.ChaosUID),
			Namespace: experimentsDetails.ChaosNamespace,
		},
		Source: apiv1.EventSource{
			Component: experimentsDetails.ChaosPodName,
		},
		Message:        eventsDetails.Message,
		Reason:         eventsDetails.Reason,
		Type:           "Normal",
		Count:          1,
		FirstTimestamp: metav1.Time{Time: time.Now()},
		LastTimestamp:  metav1.Time{Time: time.Now()},
		InvolvedObject: apiv1.ObjectReference{
			APIVersion: "litmuschaos.io/v1alpha1",
			Kind:       "ChaosEngine",
			Name:       experimentsDetails.EngineName,
			Namespace:  experimentsDetails.ChaosNamespace,
			UID:        experimentsDetails.ChaosUID,
		},
	}

	_, err := clients.KubeClient.CoreV1().Events(experimentsDetails.ChaosNamespace).Create(events)
	return err

}

//GenerateEvents update the events
func GenerateEvents(experimentsDetails *types.ExperimentDetails, clients environment.ClientSets, eventsDetails *types.EventDetails) error {

	var err error
	event, err := clients.KubeClient.CoreV1().Events(experimentsDetails.ChaosNamespace).Get(eventsDetails.Reason+string(experimentsDetails.ChaosUID), metav1.GetOptions{})

	if event.Name != eventsDetails.Reason+string(experimentsDetails.ChaosUID) {

		err = CreateEvents(experimentsDetails, clients, eventsDetails)

	} else {

		event.Count = event.Count + 1
		event.LastTimestamp = metav1.Time{Time: time.Now()}

		_, err = clients.KubeClient.CoreV1().Events(experimentsDetails.ChaosNamespace).Update(event)
	}
	return err
}
