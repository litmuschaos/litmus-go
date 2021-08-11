package environment

import (
	"strconv"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-cpu-hog-exec/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = common.Getenv("EXPERIMENT_NAME", "pod-cpu-hog")
	experimentDetails.ChaosNamespace = common.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = common.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(common.Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(common.Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.RampTime, _ = strconv.Atoi(common.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = common.Getenv("LIB", "litmus")
	experimentDetails.AppNS = common.Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = common.Getenv("APP_LABEL", "")
	experimentDetails.AppKind = common.Getenv("APP_KIND", "")
	experimentDetails.ChaosUID = clientTypes.UID(common.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = common.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = common.Getenv("POD_NAME", "")
	experimentDetails.CPUcores, _ = strconv.Atoi(common.Getenv("CPU_CORES", "1"))
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(common.Getenv("PODS_AFFECTED_PERC", "0"))
	experimentDetails.Delay, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetPods = common.Getenv("TARGET_PODS", "")
	experimentDetails.ChaosInjectCmd = common.Getenv("CHAOS_INJECT_COMMAND", "md5sum /dev/zero")
	experimentDetails.ChaosKillCmd = common.Getenv("CHAOS_KILL_COMMAND", "killall md5sum")
	experimentDetails.TargetContainer = common.Getenv("TARGET_CONTAINER", "")
	experimentDetails.Sequence = common.Getenv("SEQUENCE", "parallel")
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(common.Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))
	experimentDetails.HostNetwork = Getenvbool("HOST_NETWORK", "false")
	experimentDetails.VolMount = common.Getenv("VOL_MOUNT", "")
}

// Getenv fetch the env and set the default value for bool, if any
func Getenvbool(key string, defaultValue string) bool {
	value := os.Getenv(key)
	var val bool
	if value == "" || defaultValue == "false" {
		val = false
	} else {
		val = true
	}
	return val
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
	chaosDetails.AppDetail = appDetails
	chaosDetails.ProbeImagePullPolicy = experimentDetails.LIBImagePullPolicy
	chaosDetails.ParentsResources = []string{}
	chaosDetails.Targets = []v1alpha1.TargetDetails{}
	chaosDetails.HostNetwork = experimentDetails.HostNetwork
	chaosDetails.VolMount = experimentDetails.VolMount
}
