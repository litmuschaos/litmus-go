package types

import (
	"os"
	"strconv"
	"strings"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

const (
	// PreChaosCheck initial stage of experiment check for health before chaos injection
	PreChaosCheck string = "PreChaosCheck"
	// PostChaosCheck  pre-final stage of experiment check for health after chaos injection
	PostChaosCheck string = "PostChaosCheck"
	// Summary final stage of experiment update the verdict
	Summary string = "Summary"
	// ChaosInject this stage refer to the main chaos injection
	ChaosInject string = "ChaosInject"
	// AwaitedVerdict marked the start of test
	AwaitedVerdict string = "Awaited"
	// PassVerdict marked the verdict as passed in the end of experiment
	PassVerdict string = "Pass"
	// FailVerdict marked the verdict as failed in the end of experiment
	FailVerdict string = "Fail"
	// AbortVerdict marked the verdict as abort when experiment aborted
	AbortVerdict string = "Abort"
)

// ResultDetails is for collecting all the chaos-result-related details
type ResultDetails struct {
	Name             string
	Verdict          v1alpha1.ResultVerdict
	FailStep         string
	Phase            v1alpha1.ResultPhase
	ResultUID        clientTypes.UID
	ProbeDetails     []ProbeDetails
	PassedProbeCount int
	ProbeArtifacts   map[string]ProbeArtifact
}

// ProbeArtifact contains the probe artifacts
type ProbeArtifact struct {
	ProbeArtifacts RegisterDetails
}

// RegisterDetails contains the output of the corresponding probe
type RegisterDetails struct {
	Register string
}

// ProbeDetails is for collecting all the probe details
type ProbeDetails struct {
	Name                   string
	Type                   string
	Mode                   string
	Status                 v1alpha1.ProbeStatus
	IsProbeFailedWithError error
	RunID                  string
	RunCount               int
}

// EventDetails is for collecting all the events-related details
type EventDetails struct {
	Message      string
	Reason       string
	ResourceName string
	ResourceUID  clientTypes.UID
	Type         string
}

// ChaosDetails is for collecting all the global variables
type ChaosDetails struct {
	ChaosUID             clientTypes.UID
	ChaosNamespace       string
	ChaosPodName         string
	EngineName           string
	InstanceID           string
	ExperimentName       string
	Timeout              int
	Delay                int
	AppDetail            []AppDetails
	ChaosDuration        int
	JobCleanupPolicy     string
	ProbeImagePullPolicy string
	Randomness           bool
	Targets              []v1alpha1.TargetDetails
	ParentsResources     []ParentResource
	DefaultHealthCheck   bool
	Annotations          map[string]string
	Resources            corev1.ResourceRequirements
	ImagePullSecrets     []corev1.LocalObjectReference
	Labels               map[string]string
}

type ParentResource struct {
	Name      string
	Kind      string
	Namespace string
}

// AppDetails contains all the application related envs
type AppDetails struct {
	Namespace string
	Labels    []string
	Kind      string
	Names     []string
}

func GetTargets(targets string) []AppDetails {
	var result []AppDetails
	if targets == "" {
		return nil
	}
	t := strings.Split(targets, ";")
	for _, k := range t {
		val := strings.Split(strings.TrimSpace(k), ":")
		data := AppDetails{
			Kind:      val[0],
			Namespace: val[1],
		}
		if strings.Contains(val[2], "=") {
			data.Labels = parse(val[2])
		} else {
			data.Names = parse(val[2])
		}
		result = append(result, data)
	}
	return result
}

func parse(val string) []string {
	val = strings.TrimSpace(val)
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")
	if val == "" {
		return nil
	}
	return strings.Split(val, ",")
}

//InitialiseChaosVariables initialise all the global variables
func InitialiseChaosVariables(chaosDetails *ChaosDetails) {
	targets := Getenv("TARGETS", "")
	chaosDetails.AppDetail = GetTargets(strings.TrimSpace(targets))

	chaosDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "")
	chaosDetails.ChaosPodName = Getenv("POD_NAME", "")
	chaosDetails.Randomness, _ = strconv.ParseBool(Getenv("RANDOMNESS", ""))
	chaosDetails.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "30"))
	chaosDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	chaosDetails.EngineName = Getenv("CHAOSENGINE", "")
	chaosDetails.ExperimentName = Getenv("EXPERIMENT_NAME", "")
	chaosDetails.InstanceID = Getenv("INSTANCE_ID", "")
	chaosDetails.Timeout, _ = strconv.Atoi(Getenv("STATUS_CHECK_TIMEOUT", "180"))
	chaosDetails.Delay, _ = strconv.Atoi(Getenv("STATUS_CHECK_DELAY", "2"))
	chaosDetails.DefaultHealthCheck, _ = strconv.ParseBool(Getenv("DEFAULT_HEALTH_CHECK", "true"))
	chaosDetails.JobCleanupPolicy = Getenv("JOB_CLEANUP_POLICY", "retain")
	chaosDetails.ProbeImagePullPolicy = Getenv("LIB_IMAGE_PULL_POLICY", "Always")
	chaosDetails.ParentsResources = []ParentResource{}
	chaosDetails.Targets = []v1alpha1.TargetDetails{}
}

//SetResultAttributes initialise all the chaos result ENV
func SetResultAttributes(resultDetails *ResultDetails, chaosDetails ChaosDetails) {
	resultDetails.Verdict = "Awaited"
	resultDetails.Phase = "Running"
	resultDetails.FailStep = "N/A"
	resultDetails.PassedProbeCount = 0
	if chaosDetails.EngineName != "" {
		resultDetails.Name = chaosDetails.EngineName + "-" + chaosDetails.ExperimentName
	} else {
		resultDetails.Name = chaosDetails.ExperimentName
	}

	if chaosDetails.InstanceID != "" {
		resultDetails.Name = resultDetails.Name + "-" + chaosDetails.InstanceID
	}

}

//SetResultAfterCompletion set all the chaos result ENV in the EOT
func SetResultAfterCompletion(resultDetails *ResultDetails, verdict v1alpha1.ResultVerdict, phase v1alpha1.ResultPhase, failStep string) {
	resultDetails.Verdict = verdict
	resultDetails.Phase = phase
	resultDetails.FailStep = failStep
}

//SetEngineEventAttributes initialise attributes for event generation in chaos engine
func SetEngineEventAttributes(eventsDetails *EventDetails, Reason, Message, Type string, chaosDetails *ChaosDetails) {

	eventsDetails.Reason = Reason
	eventsDetails.Message = Message
	eventsDetails.ResourceName = chaosDetails.EngineName
	eventsDetails.ResourceUID = chaosDetails.ChaosUID
	eventsDetails.Type = Type

}

//SetResultEventAttributes initialise attributes for event generation in chaos result
func SetResultEventAttributes(eventsDetails *EventDetails, Reason, Message, Type string, resultDetails *ResultDetails) {

	eventsDetails.Reason = Reason
	eventsDetails.Message = Message
	eventsDetails.ResourceName = resultDetails.Name
	eventsDetails.ResourceUID = resultDetails.ResultUID
	eventsDetails.Type = Type
}

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}
