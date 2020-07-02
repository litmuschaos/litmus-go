package environment

import (
	"os"
	"strconv"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimenttypes "github.com/litmuschaos/litmus-go/pkg/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimenttypes.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = expName
	experimentDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.RampTime, _ = strconv.Atoi(Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = Getenv("LIB", "litmus")
	experimentDetails.ChaosServiceAccount = Getenv("CHAOS_SERVICE_ACCOUNT", "")
	experimentDetails.AppNS = Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = Getenv("APP_LABEL", "")
	experimentDetails.AppKind = Getenv("APP_KIND", "")
	experimentDetails.KillCount, _ = strconv.Atoi(Getenv("KILL_COUNT", "1"))
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	experimentDetails.AuxiliaryAppInfo = Getenv("AUXILIARY_APPINFO", "")
	experimentDetails.InstanceID = Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = Getenv("POD_NAME", "")
	experimentDetails.Force, _ = strconv.ParseBool(Getenv("FORCE", "false"))
	experimentDetails.LIBImage = Getenv("LIB_IMAGE", "")
}

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}

//InitialiseEventAttributes initialise all the event attributes
func InitialiseEventAttributes(eventsDetails *types.EventDetails, experimentDetails *experimenttypes.ExperimentDetails) {

	eventsDetails.ChaosNamespace = experimentDetails.ChaosNamespace
	eventsDetails.ChaosPodName = experimentDetails.ChaosPodName
	eventsDetails.ChaosUID = experimentDetails.ChaosUID
	eventsDetails.EngineName = experimentDetails.EngineName
	eventsDetails.ExperimentName = experimentDetails.ExperimentName
	eventsDetails.InstanceID = experimentDetails.InstanceID

}
