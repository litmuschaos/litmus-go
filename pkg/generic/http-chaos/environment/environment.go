package environment

import (
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
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
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	experimentDetails.TargetPods = types.Getenv("TARGET_PODS", "")
	experimentDetails.PodsAffectedPerc = types.Getenv("PODS_AFFECTED_PERC", "0")
	experimentDetails.NodeLabel = types.Getenv("NODE_LABEL", "")
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(types.Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "docker")
	experimentDetails.ChaosServiceAccount = types.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "/var/run/docker.sock")
	experimentDetails.SetHelperData = types.Getenv("SET_HELPER_DATA", "true")
	experimentDetails.Sequence = types.Getenv("SEQUENCE", "parallel")
	experimentDetails.NetworkInterface = types.Getenv("NETWORK_INTERFACE", "eth0")
	experimentDetails.TargetServicePort, _ = strconv.Atoi(types.Getenv("TARGET_SERVICE_PORT", "80"))
	experimentDetails.ProxyPort, _ = strconv.Atoi(types.Getenv("PROXY_PORT", "20000"))
	experimentDetails.Toxicity, _ = strconv.Atoi(types.Getenv("TOXICITY", "100"))

	switch expName {
	case "pod-http-latency":
		experimentDetails.Latency, _ = strconv.Atoi(types.Getenv("LATENCY", "6000"))
	case "pod-http-status-code":
		experimentDetails.StatusCode = types.Getenv("STATUS_CODE", "")
		experimentDetails.ModifyResponseBody = types.Getenv("MODIFY_RESPONSE_BODY", "true")
		experimentDetails.ResponseBody = types.Getenv("RESPONSE_BODY", "")
		experimentDetails.ContentType = types.Getenv("CONTENT_TYPE", "text/plain")
		experimentDetails.ContentEncoding = types.Getenv("CONTENT_ENCODING", "")
	case "pod-http-modify-header":
		experimentDetails.HeadersMap = types.Getenv("HEADERS_MAP", "{}")
		experimentDetails.HeaderMode = types.Getenv("HEADER_MODE", "response")
	case "pod-http-modify-body":
		experimentDetails.ResponseBody = types.Getenv("RESPONSE_BODY", "")
		experimentDetails.ContentType = types.Getenv("CONTENT_TYPE", "text/plain")
		experimentDetails.ContentEncoding = types.Getenv("CONTENT_ENCODING", "")
	case "pod-http-reset-peer":
		experimentDetails.ResetTimeout, _ = strconv.Atoi(types.Getenv("RESET_TIMEOUT", "0"))
	}
}
