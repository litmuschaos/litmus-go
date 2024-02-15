package types

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
	"github.com/palantir/stacktrace"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

var err error

const (
	SideCarEnabled = "sidecar/enabled"
	SideCarPrefix  = "SIDECAR"
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
	// ErrorVerdict marked the verdict as error in the end of experiment
	ErrorVerdict string = "Error"
)

type ExperimentPhase string

const (
	PreChaosPhase    ExperimentPhase = "PreChaos"
	PostChaosPhase   ExperimentPhase = "PostChaos"
	ChaosInjectPhase ExperimentPhase = "ChaosInject"
)

// ResultDetails is for collecting all the chaos-result-related details
type ResultDetails struct {
	Name             string
	Verdict          v1alpha1.ResultVerdict
	ErrorOutput      *v1alpha1.ErrorOutput
	Phase            v1alpha1.ResultPhase
	ResultUID        clientTypes.UID
	ProbeDetails     []*ProbeDetails
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
	Failed                 bool
	HasProbeCompleted      bool
	RunID                  string
	RunCount               int
	Stopped                bool
	Timeouts               ProbeTimeouts
}

type ProbeTimeouts struct {
	ProbeTimeout         time.Duration
	Interval             time.Duration
	ProbePollingInterval time.Duration
	InitialDelay         time.Duration
	EvaluationTimeout    time.Duration
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
	Phase                ExperimentPhase
	ProbeContext         ProbeContext
	SideCar              []SideCar
}

type SideCar struct {
	ENV             []corev1.EnvVar
	Image           string
	ImagePullPolicy corev1.PullPolicy
	Name            string
	Secrets         []v1alpha1.Secret
	EnvFrom         []corev1.EnvFromSource
}

type ParentResource struct {
	Name      string
	Kind      string
	Namespace string
}

type ProbeContext struct {
	Ctx        context.Context
	CancelFunc context.CancelFunc
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

// InitialiseChaosVariables initialise all the global variables
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
	chaosDetails.Phase = PreChaosPhase
	chaosDetails.ProbeContext.Ctx, chaosDetails.ProbeContext.CancelFunc = context.WithCancel(context.Background())
	chaosDetails.Labels = map[string]string{}
}

// SetResultAttributes initialise all the chaos result ENV
func SetResultAttributes(resultDetails *ResultDetails, chaosDetails ChaosDetails) {
	resultDetails.Verdict = "Awaited"
	resultDetails.Phase = "Running"
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

// SetResultAfterCompletion set all the chaos result ENV in the EOT
func SetResultAfterCompletion(resultDetails *ResultDetails, verdict v1alpha1.ResultVerdict, phase v1alpha1.ResultPhase, failStep string, errorCode cerrors.ErrorType) {
	resultDetails.Verdict = verdict
	resultDetails.Phase = phase
	if errorCode != cerrors.ErrorTypeHelperPodFailed && resultDetails.Phase == v1alpha1.ResultPhaseError {
		resultDetails.ErrorOutput = &v1alpha1.ErrorOutput{
			Reason:    failStep,
			ErrorCode: string(errorCode),
		}
	}
}

// SetEngineEventAttributes initialise attributes for event generation in chaos engine
func SetEngineEventAttributes(eventsDetails *EventDetails, Reason, Message, Type string, chaosDetails *ChaosDetails) {

	eventsDetails.Reason = Reason
	eventsDetails.Message = Message
	eventsDetails.ResourceName = chaosDetails.EngineName
	eventsDetails.ResourceUID = chaosDetails.ChaosUID
	eventsDetails.Type = Type

}

// SetResultEventAttributes initialise attributes for event generation in chaos result
func SetResultEventAttributes(eventsDetails *EventDetails, Reason, Message, Type string, resultDetails *ResultDetails) {

	eventsDetails.Reason = Reason
	eventsDetails.Message = Message
	eventsDetails.ResourceName = resultDetails.Name
	eventsDetails.ResourceUID = resultDetails.ResultUID
	eventsDetails.Type = Type
}

// GetChaosResultVerdictEvent return the verdict and event type
func GetChaosResultVerdictEvent(verdict v1alpha1.ResultVerdict) (string, string) {
	switch verdict {
	case v1alpha1.ResultVerdictPassed:
		return string(verdict), "Normal"
	default:
		return string(verdict), "Warning"
	}
}

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}

// GetChaosEngine fetches the chaosengine instance
func GetChaosEngine(chaosDetails *ChaosDetails, clients clients.ClientSets) (*v1alpha1.ChaosEngine, error) {
	var engine *v1alpha1.ChaosEngine

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

// GetValuesFromChaosEngine get the values from the chaosengine
func GetValuesFromChaosEngine(chaosDetails *ChaosDetails, clients clients.ClientSets, chaosresult *ResultDetails) error {

	// get the chaosengine instance
	engine, err := GetChaosEngine(chaosDetails, clients)
	if err != nil {
		return stacktrace.Propagate(err, "could not get chaosengine")
	}

	// get all the probes defined inside chaosengine for the corresponding experiment
	for _, experiment := range engine.Spec.Experiments {
		if experiment.Name == chaosDetails.ExperimentName {
			if err := InitializeProbesInChaosResultDetails(chaosresult, experiment.Spec.Probe); err != nil {
				return stacktrace.Propagate(err, "could not initialize probe")
			}
			InitializeSidecarDetails(chaosDetails, engine, experiment.Spec.Components.ENV)
		}
	}

	return nil
}

// InitializeSidecarDetails sets the sidecar details
func InitializeSidecarDetails(chaosDetails *ChaosDetails, engine *v1alpha1.ChaosEngine, env []corev1.EnvVar) {
	if engine.Annotations == nil {
		return
	}

	if sidecarEnabled, ok := engine.Annotations[SideCarEnabled]; !ok || (sidecarEnabled == "false") {
		return
	}

	if len(engine.Spec.Components.Sidecar) == 0 {
		return
	}

	var sidecars []SideCar
	for _, v := range engine.Spec.Components.Sidecar {
		sidecar := SideCar{
			Image:           v.Image,
			ImagePullPolicy: v.ImagePullPolicy,
			Name:            chaosDetails.ExperimentName + "-sidecar-" + stringutils.GetRunID(),
			Secrets:         v.Secrets,
			ENV:             append(v.ENV, getDefaultEnvs(chaosDetails.ExperimentName)...),
			EnvFrom:         v.EnvFrom,
		}

		if sidecar.ImagePullPolicy == "" {
			sidecar.ImagePullPolicy = corev1.PullIfNotPresent
		}

		sidecars = append(sidecars, sidecar)
	}

	chaosDetails.SideCar = sidecars
}

func getDefaultEnvs(cName string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:      "POD_NAME",
			ValueFrom: getEnvSource("v1", "metadata.name"),
		},
		{
			Name:      "POD_NAMESPACE",
			ValueFrom: getEnvSource("v1", "metadata.namespace"),
		},
		{
			Name:  "MAIN_CONTAINER",
			Value: cName,
		},
	}
}

// getEnvSource return the env source for the given apiVersion & fieldPath
func getEnvSource(apiVersion string, fieldPath string) *corev1.EnvVarSource {
	downwardENV := corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: apiVersion,
			FieldPath:  fieldPath,
		},
	}
	return &downwardENV
}

func InitializeProbesInChaosResultDetails(chaosresult *ResultDetails, probes []v1alpha1.ProbeAttributes) error {
	var probeDetails []*ProbeDetails

	// set the probe details for k8s probe
	for _, probe := range probes {
		tempProbe := &ProbeDetails{}
		tempProbe.Name = probe.Name
		tempProbe.Type = probe.Type
		tempProbe.Mode = probe.Mode
		tempProbe.RunCount = 0
		tempProbe.Status = v1alpha1.ProbeStatus{
			Verdict: "Awaited",
		}
		tempProbe.Timeouts, err = parseProbeTimeouts(probe)
		if err != nil {
			return err
		}
		probeDetails = append(probeDetails, tempProbe)
	}

	chaosresult.ProbeDetails = probeDetails
	chaosresult.ProbeArtifacts = map[string]ProbeArtifact{}
	return nil
}

func parseProbeTimeouts(probe v1alpha1.ProbeAttributes) (ProbeTimeouts, error) {
	var timeout ProbeTimeouts
	timeout.ProbeTimeout, err = parseDuration(probe.RunProperties.ProbeTimeout)
	if err != nil {
		return timeout, generateError(probe.Name, probe.Type, "ProbeTimeout", err)
	}
	timeout.Interval, err = parseDuration(probe.RunProperties.Interval)
	if err != nil {
		return timeout, generateError(probe.Name, probe.Type, "Interval", err)
	}
	timeout.ProbePollingInterval, err = parseDuration(probe.RunProperties.ProbePollingInterval)
	if err != nil {
		return timeout, generateError(probe.Name, probe.Type, "ProbePollingInterval", err)
	}
	timeout.InitialDelay, err = parseDuration(probe.RunProperties.InitialDelay)
	if err != nil {
		return timeout, generateError(probe.Name, probe.Type, "InitialDelay", err)
	}
	timeout.EvaluationTimeout, err = parseDuration(probe.RunProperties.EvaluationTimeout)
	if err != nil {
		return timeout, generateError(probe.Name, probe.Type, "EvaluationTimeout", err)
	}
	return timeout, nil
}

func parseDuration(duration string) (time.Duration, error) {
	if strings.TrimSpace(duration) == "" {
		return 0, nil
	}
	return time.ParseDuration(duration)
}

func generateError(probeName, probeType, field string, err error) error {
	return cerrors.Error{
		ErrorCode: cerrors.ErrorTypeGeneric,
		Reason:    fmt.Sprintf("Invalid probe runProperties field '%s': %s", field, err.Error()),
		Target:    fmt.Sprintf("{probeName: %s, type: %s}", probeName, probeType),
	}
}
