package environment

import (
	"os"
	"strconv"

	clientTypes "k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = expName
	experimentDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "shubham")
	experimentDetails.EngineName = Getenv("CHAOSENGINE", "probe")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.RampTime, _ = strconv.Atoi(Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = Getenv("LIB", "litmus")
	experimentDetails.ChaosServiceAccount = Getenv("CHAOS_SERVICE_ACCOUNT", "litmus")
	experimentDetails.AppNS = Getenv("APP_NAMESPACE", "shubham")
	experimentDetails.AppLabel = Getenv("APP_LABEL", "run=nginx")
	experimentDetails.AppKind = Getenv("APP_KIND", "")
	experimentDetails.KillCount, _ = strconv.Atoi(Getenv("KILL_COUNT", "1"))
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", "380f8672-6826-4231-917f-2ebcb9393a49"))
	experimentDetails.InstanceID = Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = Getenv("POD_NAME", "hello")
	experimentDetails.Force, _ = strconv.ParseBool(Getenv("FORCE", "false"))
	experimentDetails.LIBImage = Getenv("LIB_IMAGE", "litmuschaos/pod-delete-helper:latest")
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

}
