package environment

import (
	"os"
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
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
	experimentDetails.ChaosLib = Getenv("LIB", "pumba")
	experimentDetails.AppNS = Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = Getenv("APP_LABEL", "")
	experimentDetails.AppKind = Getenv("APP_KIND", "")
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = Getenv("INSTANCE_ID", "")
	experimentDetails.LIBImage = Getenv("LIB_IMAGE", "litmuschaos/go-runner:latest")
	experimentDetails.ChaosPodName = Getenv("POD_NAME", "")
	experimentDetails.RunID = Getenv("RunID", "")
	experimentDetails.NetworkPacketDuplicationPercentage, _ = strconv.Atoi(Getenv("NETWORK_PACKET_DUPLICATION_PERCENTAGE", "100"))
	experimentDetails.NetworkLatency, _ = strconv.Atoi(Getenv("NETWORK_LATENCY", "60000"))
	experimentDetails.NetworkPacketLossPercentage, _ = strconv.Atoi(Getenv("NETWORK_PACKET_LOSS_PERCENTAGE", "100"))
	experimentDetails.NetworkPacketCorruptionPercentage, _ = strconv.Atoi(Getenv("NETWORK_PACKET_CORRUPTION_PERCENTAGE", "100"))
	experimentDetails.NetworkInterface = Getenv("NETWORK_INTERFACE", "eth0")
	experimentDetails.TargetContainer = Getenv("TARGET_CONTAINER", "")
	experimentDetails.TCImage = Getenv("TC_IMAGE", "gaiadocker/iproute2")
	experimentDetails.Delay, _ = strconv.Atoi(Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetPod = Getenv("TARGET_POD", "")
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(Getenv("PODS_AFFECTED_PERC", "0"))
	experimentDetails.TargetIPs = Getenv("TARGET_IPs", "")
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
