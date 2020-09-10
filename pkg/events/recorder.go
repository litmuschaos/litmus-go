package events

import (
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	litmuschaosScheme "github.com/litmuschaos/chaos-operator/pkg/client/clientset/versioned/scheme"
)

// Recorder is collection of resources needed to record events for chaos-runner
type Recorder struct {
	EventRecorder record.EventRecorder
	EventResource runtime.Object
}

func generateEventRecorder(kubeClient *kubernetes.Clientset, componentName string) (record.EventRecorder, error) {
	err := litmuschaosScheme.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: componentName})
	return recorder, nil
}

// NewEventRecorder initializes EventRecorder with Resource as ChaosEngine
func NewEventRecorder(clients clients.ClientSets, chaosDetails types.ChaosDetails) (*Recorder, error) {
	engineForEvent, err := GetChaosEngine(clients, chaosDetails.ChaosNamespace, chaosDetails.EngineName)
	if err != nil {
		return &Recorder{}, err
	}
	eventBroadCaster, err := generateEventRecorder(clients.KubeClient, chaosDetails.ChaosPodName)
	if err != nil {
		return &Recorder{}, err
	}
	return &Recorder{
		EventRecorder: eventBroadCaster,
		EventResource: engineForEvent,
	}, nil
}

// PreChaosCheck is an standard event spawned just after ApplicationStatusCheck
func (r Recorder) PreChaosCheck() {
	r.EventRecorder.Eventf(r.EventResource, corev1.EventTypeNormal, types.PreChaosCheck, "AUT is Running successfully")
	time.Sleep(5 * time.Second)
}

// PostChaosCheck is an standard event spawned just after ApplicationStatusCheck
func (r Recorder) PostChaosCheck() {
	r.EventRecorder.Eventf(r.EventResource, corev1.EventTypeNormal, types.PostChaosCheck, "AUT is Running successfully")
	time.Sleep(5 * time.Second)
}

// ChaosInject is an standard event spawned just after chaos injection
func (r Recorder) ChaosInject(ExperimentName string) {
	r.EventRecorder.Eventf(r.EventResource, corev1.EventTypeNormal, types.ChaosInject, "Injecting %v chaos on application pod", ExperimentName)
	time.Sleep(5 * time.Second)
}

// Summary is an standard event spawned in the end of test
func (r Recorder) Summary(ExperimentName string, resultDetails *types.ResultDetails) {
	r.EventRecorder.Eventf(r.EventResource, corev1.EventTypeNormal, types.Summary, "%v experiment has been %ved", ExperimentName, resultDetails.Verdict)
	time.Sleep(5 * time.Second)
}

// GetChaosEngine returns chaosEngine Object
func GetChaosEngine(clients clients.ClientSets, ChaosNamespace string, EngineName string) (*v1alpha1.ChaosEngine, error) {
	expEngine, err := clients.LitmusClient.ChaosEngines(ChaosNamespace).Get(EngineName, metav1.GetOptions{})
	if err != nil {

		return nil, errors.Wrapf(err, "Unable to get ChaosEngine Name: %v, in namespace: %v, due to error: %v", EngineName, ChaosNamespace, err)
	}
	return expEngine, nil
}
