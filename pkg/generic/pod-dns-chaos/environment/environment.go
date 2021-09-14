package environment

import (
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-dns-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// DNSChaosType represents the DNS chaos type
type DNSChaosType string

const (
	// Error represents DNS error
	Error DNSChaosType = "error"
	// Spoof represents DNS spoofing
	Spoof DNSChaosType = "spoof"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails, expType DNSChaosType) {
	experimentDetails.ChaosNamespace = common.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = common.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(common.Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.RampTime, _ = strconv.Atoi(common.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = common.Getenv("LIB", "litmus")
	experimentDetails.AppNS = common.Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = common.Getenv("APP_LABEL", "")
	experimentDetails.AppKind = common.Getenv("APP_KIND", "")
	experimentDetails.ChaosUID = clientTypes.UID(common.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = common.Getenv("INSTANCE_ID", "")
	experimentDetails.LIBImage = common.Getenv("LIB_IMAGE", "litmuschaos/go-runner:latest")
	experimentDetails.LIBImagePullPolicy = common.Getenv("LIB_IMAGE_PULL_POLICY", "Always")
	experimentDetails.TargetContainer = common.Getenv("TARGET_CONTAINER", "")
	experimentDetails.ChaosPodName = common.Getenv("POD_NAME", "")
	experimentDetails.Delay, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetPods = common.Getenv("TARGET_PODS", "")
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(common.Getenv("PODS_AFFECTED_PERC", "0"))
	experimentDetails.ContainerRuntime = common.Getenv("CONTAINER_RUNTIME", "docker")
	experimentDetails.SocketPath = common.Getenv("SOCKET_PATH", "/var/run/docker.sock")
	experimentDetails.ChaosServiceAccount = common.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	experimentDetails.Sequence = common.Getenv("SEQUENCE", "parallel")
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(common.Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))
	switch expType {
	case Error:
		experimentDetails.ExperimentName = common.Getenv("EXPERIMENT_NAME", "pod-dns-error")
		experimentDetails.TargetHostNames = common.Getenv("TARGET_HOSTNAMES", "")
		experimentDetails.MatchScheme = common.Getenv("MATCH_SCHEME", "exact")
		experimentDetails.ChaosType = common.Getenv("CHAOS_TYPE", "error")
	case Spoof:
		experimentDetails.ExperimentName = common.Getenv("EXPERIMENT_NAME", "pod-dns-spoof")
		experimentDetails.SpoofMap = common.Getenv("SPOOF_MAP", "")
		experimentDetails.ChaosType = common.Getenv("CHAOS_TYPE", "spoof")
	}
}

//InitialiseChaosVariables initialise all the global variables
func InitialiseChaosVariables(chaosDetails *types.ChaosDetails, experimentDetails *experimentTypes.ExperimentDetails) {
	appDetails := types.AppDetails{}
	appDetails.AnnotationCheck, _ = strconv.ParseBool(common.Getenv("ANNOTATION_CHECK", "false"))
	appDetails.AnnotationKey = common.Getenv("ANNOTATION_KEY", "litmuschaos.io/chaos")
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
	chaosDetails.JobCleanupPolicy = common.Getenv("JOB_CLEANUP_POLICY", "retain")
	chaosDetails.ProbeImagePullPolicy = experimentDetails.LIBImagePullPolicy
	chaosDetails.DefaultAppHealthCheck, _ = strconv.ParseBool(common.Getenv("DEFAULT_APP_HEALTH_CHECK", "true"))
}
