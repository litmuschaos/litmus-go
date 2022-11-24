package environment

import (
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.LIBImage = types.Getenv("LIB_IMAGE", "litmuschaos/go-runner:latest")
	experimentDetails.LIBImagePullPolicy = types.Getenv("LIB_IMAGE_PULL_POLICY", "Always")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.NetworkInterface = types.Getenv("NETWORK_INTERFACE", "eth0")
	experimentDetails.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetPods = types.Getenv("TARGET_PODS", "")
	experimentDetails.PodsAffectedPerc = types.Getenv("PODS_AFFECTED_PERC", "0")
	experimentDetails.NodeLabel = types.Getenv("NODE_LABEL", "")
	experimentDetails.DestinationIPs = types.Getenv("DESTINATION_IPS", "")
	experimentDetails.DestinationHosts = types.Getenv("DESTINATION_HOSTS", "")
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "docker")
	experimentDetails.ChaosServiceAccount = types.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "/var/run/docker.sock")
	experimentDetails.Sequence = types.Getenv("SEQUENCE", "parallel")
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(types.Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))
	experimentDetails.SetHelperData = types.Getenv("SET_HELPER_DATA", "true")
	experimentDetails.SourcePorts = types.Getenv("SOURCE_PORTS", "")
	experimentDetails.DestinationPorts = types.Getenv("DESTINATION_PORTS", "")

	switch expName {
	case "pod-network-loss":
		experimentDetails.NetworkPacketLossPercentage = types.Getenv("NETWORK_PACKET_LOSS_PERCENTAGE", "100")
		experimentDetails.NetworkChaosType = "network-loss"

	case "pod-network-latency":
		experimentDetails.NetworkLatency, _ = strconv.Atoi(types.Getenv("NETWORK_LATENCY", "2000"))
		experimentDetails.Jitter, _ = strconv.Atoi(types.Getenv("JITTER", "0"))
		experimentDetails.NetworkChaosType = "network-latency"

	case "pod-network-corruption":
		experimentDetails.NetworkPacketCorruptionPercentage = types.Getenv("NETWORK_PACKET_CORRUPTION_PERCENTAGE", "100")
		experimentDetails.NetworkChaosType = "network-corruption"

	case "pod-network-duplication":
		experimentDetails.NetworkPacketDuplicationPercentage = types.Getenv("NETWORK_PACKET_DUPLICATION_PERCENTAGE", "100")
		experimentDetails.NetworkChaosType = "network-duplication"
	}
}
