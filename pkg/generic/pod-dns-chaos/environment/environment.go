package environment

import (
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-dns-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
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
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.LIBImage = types.Getenv("LIB_IMAGE", "litmuschaos/go-runner:latest")
	experimentDetails.LIBImagePullPolicy = types.Getenv("LIB_IMAGE_PULL_POLICY", "Always")
	experimentDetails.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetPods = types.Getenv("TARGET_PODS", "")
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(types.Getenv("PODS_AFFECTED_PERC", "0"))
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "docker")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "/var/run/docker.sock")
	experimentDetails.ChaosServiceAccount = types.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	experimentDetails.Sequence = types.Getenv("SEQUENCE", "parallel")
	experimentDetails.SetHelperData = types.Getenv("SET_HELPER_DATA", "true")
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(types.Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))
	switch expType {
	case Error:
		experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "pod-dns-error")
		experimentDetails.TargetHostNames = types.Getenv("TARGET_HOSTNAMES", "")
		experimentDetails.MatchScheme = types.Getenv("MATCH_SCHEME", "exact")
		experimentDetails.ChaosType = types.Getenv("CHAOS_TYPE", "error")
	case Spoof:
		experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "pod-dns-spoof")
		experimentDetails.SpoofMap = types.Getenv("SPOOF_MAP", "")
		experimentDetails.ChaosType = types.Getenv("CHAOS_TYPE", "spoof")
	}
}
