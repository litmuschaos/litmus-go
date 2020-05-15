package events

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/environment"
	"time"
	"github.com/pkg/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"

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

// NewEventRecorder initalizes EventRecorder with Resource as ChaosEngine
func NewEventRecorder(clients environment.ClientSets, experimentsDetails types.ExperimentDetails) (*Recorder, error) {
	engineForEvent, err := GetChaosEngine(clients,experimentsDetails)
	if err != nil {
		return &Recorder{}, err
	}
	eventBroadCaster, err := generateEventRecorder(clients.KubeClient, experimentsDetails.ChaosPodName)
	if err != nil {
		return &Recorder{}, err
	}
	return &Recorder{
		EventRecorder: eventBroadCaster,
		EventResource: engineForEvent,
	}, nil
}

// PreChaosCheck is an standard event spawned just after ApplicationStatusCheck
func (r Recorder) PreChaosCheck(experimentsDetails types.ExperimentDetails) {
	r.EventRecorder.Eventf(r.EventResource, corev1.EventTypeNormal, types.PreChaosCheck, "AUT is Running successfully")
	time.Sleep(5 * time.Second)
}

// PostChaosCheck is an standard event spawned just after ApplicationStatusCheck
func (r Recorder) PostChaosCheck(experimentsDetails types.ExperimentDetails) {
	r.EventRecorder.Eventf(r.EventResource, corev1.EventTypeNormal, types.PostChaosCheck, "AUT is Running successfully")
	time.Sleep(5 * time.Second)
}

// ChaosInject is an standard event spawned just after chaos injection
func (r Recorder) ChaosInject(experimentsDetails *types.ExperimentDetails) {
	r.EventRecorder.Eventf(r.EventResource, corev1.EventTypeNormal, types.ChaosInject, "Injecting %v chaos on application pod",experimentsDetails.ExperimentName)
	time.Sleep(5 * time.Second)
}

// Summary is an standard event spawned in the end of test
func (r Recorder) Summary(experimentsDetails *types.ExperimentDetails, resultDetails *types.ResultDetails) {
	r.EventRecorder.Eventf(r.EventResource, corev1.EventTypeNormal, types.Summary, "%v experiment has been %ved",experimentsDetails.ExperimentName, resultDetails.Verdict)
	time.Sleep(5 * time.Second)
}

// GetChaosEngine returns chaosEngine Object
func GetChaosEngine(clients environment.ClientSets, experimentsDetails types.ExperimentDetails) (*v1alpha1.ChaosEngine, error) {
	expEngine, err := clients.LitmusClient.ChaosEngines(experimentsDetails.ChaosNamespace).Get(experimentsDetails.EngineName, metav1.GetOptions{})
	if err != nil {

		return nil, errors.Wrapf(err, "Unable to get ChaosEngine Name: %v, in namespace: %v, due to error: %v", experimentsDetails.EngineName, experimentsDetails.ChaosNamespace, err)
	}
	return expEngine, nil
}
