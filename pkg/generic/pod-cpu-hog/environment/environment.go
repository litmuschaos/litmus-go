package environment

import (
	"os"
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-cpu-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = Getenv("EXPERIMENT_NAME", "pod-cpu-hog")
	experimentDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.RampTime, _ = strconv.Atoi(Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = Getenv("LIB", "litmus")
	experimentDetails.AppNS = Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = Getenv("APP_LABEL", "")
	experimentDetails.AppKind = Getenv("APP_KIND", "")
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = Getenv("POD_NAME", "")
	experimentDetails.CPUcores, _ = strconv.Atoi(Getenv("CPU_CORES", "1"))
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(Getenv("PODS_AFFECTED_PERC", "0"))
	experimentDetails.Delay, _ = strconv.Atoi(Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetPod = Getenv("TARGET_POD", "")
	experimentDetails.ChaosInjectCmd = Getenv("CHAOS_INJECT_COMMAND", "md5sum /dev/zero")
	experimentDetails.ChaosKillCmd = Getenv("CHAOS_KILL_COMMAND", "kill $(find /proc -name exe -lname '*/md5sum' 2>&1 | grep -v 'Permission denied' | awk -F/ '{print $(NF-1)}' |  head -n 1)")
	experimentDetails.LIBImage = Getenv("LIB_IMAGE", "gaiaadm/pumba")
	experimentDetails.TargetContainer = Getenv("TARGET_CONTAINER", "")
	experimentDetails.Sequence = Getenv("SEQUENCE", "parallel")

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

	chaosDetails.ChaosNamespace = experimentDetails.ChaosNamespace
	chaosDetails.ChaosPodName = experimentDetails.ChaosPodName
	chaosDetails.ChaosUID = experimentDetails.ChaosUID
	chaosDetails.EngineName = experimentDetails.EngineName
	chaosDetails.ExperimentName = experimentDetails.ExperimentName
	chaosDetails.InstanceID = experimentDetails.InstanceID
	chaosDetails.Timeout = experimentDetails.Timeout
	chaosDetails.Delay = experimentDetails.Delay
}
