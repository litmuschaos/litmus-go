package events

import (
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/types"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//CreateEvents create the events in the desired resource
func CreateEvents(eventsDetails *types.EventDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, kind string) error {

	events := &apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      eventsDetails.Reason + chaosDetails.ExperimentName + string(chaosDetails.ChaosUID),
			Namespace: chaosDetails.ChaosNamespace,
		},
		Source: apiv1.EventSource{
			Component: chaosDetails.ChaosPodName,
		},
		Message:        eventsDetails.Message,
		Reason:         eventsDetails.Reason,
		Type:           eventsDetails.Type,
		Count:          1,
		FirstTimestamp: metav1.Time{Time: time.Now()},
		LastTimestamp:  metav1.Time{Time: time.Now()},
		InvolvedObject: apiv1.ObjectReference{
			APIVersion: "litmuschaos.io/v1alpha1",
			Kind:       kind,
			Name:       eventsDetails.ResourceName,
			Namespace:  chaosDetails.ChaosNamespace,
			UID:        eventsDetails.ResourceUID,
		},
	}

	_, err := clients.KubeClient.CoreV1().Events(chaosDetails.ChaosNamespace).Create(events)
	return err

}

//GenerateEvents update the events and increase the count by 1, if already present
// else it will create a new event
func GenerateEvents(eventsDetails *types.EventDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, kind string) error {

	var err error
	event, _ := clients.KubeClient.CoreV1().Events(chaosDetails.ChaosNamespace).Get(eventsDetails.Reason+string(eventsDetails.ResourceUID), metav1.GetOptions{})

	if event.Name != eventsDetails.Reason+chaosDetails.ExperimentName+string(chaosDetails.ChaosUID) {

		err = CreateEvents(eventsDetails, clients, chaosDetails, kind)

	} else {

		event.LastTimestamp = metav1.Time{Time: time.Now()}

		if kind == "ChaosResult" {
			event.Message = eventsDetails.Message
			event.CreationTimestamp = metav1.Time{Time: time.Now()}
			event.FirstTimestamp = metav1.Time{Time: time.Now()}
			event.Source.Component = chaosDetails.ChaosPodName
			event.Type = eventsDetails.Type
		} else {
			event.Count = event.Count + 1
		}

		_, err = clients.KubeClient.CoreV1().Events(chaosDetails.ChaosNamespace).Update(event)
	}
	return err
}
