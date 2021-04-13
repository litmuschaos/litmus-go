package environment

import (
	"os"
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-dns-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = Getenv("EXPERIMENT_NAME", "")
	experimentDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.RampTime, _ = strconv.Atoi(Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = Getenv("LIB", "litmus")
	experimentDetails.AppNS = Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = Getenv("APP_LABEL", "")
	experimentDetails.AppKind = Getenv("APP_KIND", "")
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = Getenv("INSTANCE_ID", "")
	experimentDetails.LIBImage = Getenv("LIB_IMAGE", "litmuschaos/go-runner:latest")
	experimentDetails.LIBImagePullPolicy = Getenv("LIB_IMAGE_PULL_POLICY", "Always")
	experimentDetails.TargetContainer = Getenv("TARGET_CONTAINER", "")
	experimentDetails.ChaosPodName = Getenv("POD_NAME", "")
	experimentDetails.Delay, _ = strconv.Atoi(Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetPods = Getenv("TARGET_PODS", "")
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(Getenv("PODS_AFFECTED_PERC", "0"))
	experimentDetails.TargetHostNames = Getenv("TARGET_HOSTNAMES", "")
	experimentDetails.MatchScheme = Getenv("MATCH_SCHEME", "exact")
	experimentDetails.ChaosType = Getenv("CHAOS_TYPE", "error")
	experimentDetails.ContainerRuntime = Getenv("CONTAINER_RUNTIME", "docker")
	experimentDetails.SocketPath = Getenv("SOCKET_PATH", "/var/run/docker.sock")
	experimentDetails.ChaosServiceAccount = Getenv("CHAOS_SERVICE_ACCOUNT", "")
	experimentDetails.Sequence = Getenv("SEQUENCE", "parallel")
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))
}

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}

//InitialiseChaosVariables initialise all the global variables
func InitialiseChaosVariables(chaosDetails *types.ChaosDetails, experimentDetails *experimentTypes.ExperimentDetails) {
	appDetails := types.AppDetails{}
	appDetails.AnnotationCheck, _ = strconv.ParseBool(Getenv("ANNOTATION_CHECK", "false"))
	appDetails.AnnotationKey = Getenv("ANNOTATION_KEY", "litmuschaos.io/chaos")
	appDetails.AnnotationValue = "true"
	appDetails.Kind = experimentDetails.AppKind
	appDetails.Label = experimentDetails.AppLabel
	appDetails.Namespace = experimentDetails.AppNS

	chaosDetails.ChaosNamespace = experimentDetails.ChaosNamespace
	chaosDetails.ChaosPodName = experimentDetails.ChaosPodName
	chaosDetails.ChaosUID = experimentDetails.ChaosUID
	chaosDetails.EngineName = experimentDetails.EngineName
	chaosDetails.ExperimentName = experimentDetails.ExperimentName
	chaosDetails.InstanceID = experimentDetails.InstanceID
	chaosDetails.Timeout = experimentDetails.Timeout
	chaosDetails.Delay = experimentDetails.Delay
	chaosDetails.ChaosDuration = experimentDetails.ChaosDuration
	chaosDetails.AppDetail = appDetails
	chaosDetails.JobCleanupPolicy = Getenv("JOB_CLEANUP_POLICY", "retain")
	chaosDetails.ProbeImagePullPolicy = experimentDetails.LIBImagePullPolicy
}
