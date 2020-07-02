package events

import (
	"time"

	"github.com/litmuschaos/litmus-go/pkg/environment"
	"github.com/litmuschaos/litmus-go/pkg/types"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//CreateEvents create the events
func CreateEvents(eventsDetails *types.EventDetails, clients environment.ClientSets) error {

	events := &apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      eventsDetails.Reason + string(eventsDetails.ChaosUID),
			Namespace: eventsDetails.ChaosNamespace,
		},
		Source: apiv1.EventSource{
			Component: eventsDetails.ChaosPodName,
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
			Name:       eventsDetails.EngineName,
			Namespace:  eventsDetails.ChaosNamespace,
			UID:        eventsDetails.ChaosUID,
		},
	}

	_, err := clients.KubeClient.CoreV1().Events(eventsDetails.ChaosNamespace).Create(events)
	return err

}

//GenerateEvents update the events
func GenerateEvents(eventsDetails *types.EventDetails, clients environment.ClientSets) error {

	var err error
	event, err := clients.KubeClient.CoreV1().Events(eventsDetails.ChaosNamespace).Get(eventsDetails.Reason+string(eventsDetails.ChaosUID), metav1.GetOptions{})

	if event.Name != eventsDetails.Reason+string(eventsDetails.ChaosUID) {

		err = CreateEvents(eventsDetails, clients)

	} else {

		event.Count = event.Count + 1
		event.LastTimestamp = metav1.Time{Time: time.Now()}

		_, err = clients.KubeClient.CoreV1().Events(eventsDetails.ChaosNamespace).Update(event)
	}
	return err
}
