package environment

import (
	"os"
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/pod-network-duplication/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = expName
	experimentDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.RampTime, _ = strconv.Atoi(Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = Getenv("LIB", "litmus")
	experimentDetails.ChaosServiceAccount = Getenv("CHAOS_SERVICE_ACCOUNT", "")
	experimentDetails.AppNS = Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = Getenv("APP_LABEL", "")
	experimentDetails.AppKind = Getenv("APP_KIND", "")
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	experimentDetails.AuxiliaryAppInfo = Getenv("AUXILIARY_APPINFO", "")
	experimentDetails.InstanceID = Getenv("INSTANCE_ID", "")
	experimentDetails.LIBImage = Getenv("LIB_IMAGE", "")
	experimentDetails.ChaosPodName = Getenv("POD_NAME", "")
	experimentDetails.NetworkPacketDuplicationPercentage, _ = strconv.Atoi(Getenv("NETWORK_PACKET_DUPLICATION_PERCENTAGE", "80"))
	experimentDetails.NetworkInterface = Getenv("NETWORK_INTERFACE", "eth0")
	experimentDetails.TargetContainer = Getenv("TARGET_CONTAINER", "")
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
func InitialiseEventAttributes(eventsDetails *types.ChaosDetails, experimentDetails *experimentTypes.ExperimentDetails) {

	eventsDetails.ChaosNamespace = experimentDetails.ChaosNamespace
	eventsDetails.ChaosPodName = experimentDetails.ChaosPodName
	eventsDetails.ChaosUID = experimentDetails.ChaosUID
	eventsDetails.EngineName = experimentDetails.EngineName
	eventsDetails.ExperimentName = experimentDetails.ExperimentName
	eventsDetails.InstanceID = experimentDetails.InstanceID

}
