package environment

import (
	"strconv"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = common.Getenv("EXPERIMENT_NAME", "")
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
	experimentDetails.ChaosPodName = common.Getenv("POD_NAME", "")
	experimentDetails.NetworkPacketDuplicationPercentage, _ = strconv.Atoi(common.Getenv("NETWORK_PACKET_DUPLICATION_PERCENTAGE", "100"))
	experimentDetails.NetworkLatency, _ = strconv.Atoi(common.Getenv("NETWORK_LATENCY", "60000"))
	experimentDetails.NetworkPacketLossPercentage, _ = strconv.Atoi(common.Getenv("NETWORK_PACKET_LOSS_PERCENTAGE", "100"))
	experimentDetails.NetworkPacketCorruptionPercentage, _ = strconv.Atoi(common.Getenv("NETWORK_PACKET_CORRUPTION_PERCENTAGE", "100"))
	experimentDetails.NetworkInterface = common.Getenv("NETWORK_INTERFACE", "eth0")
	experimentDetails.TargetContainer = common.Getenv("TARGET_CONTAINER", "")
	experimentDetails.TCImage = common.Getenv("TC_IMAGE", "gaiadocker/iproute2")
	experimentDetails.Delay, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetPods = common.Getenv("TARGET_PODS", "")
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(common.Getenv("PODS_AFFECTED_PERC", "0"))
	experimentDetails.DestinationIPs = common.Getenv("DESTINATION_IPS", "")
	experimentDetails.DestinationHosts = common.Getenv("DESTINATION_HOSTS", "")
	experimentDetails.ContainerRuntime = common.Getenv("CONTAINER_RUNTIME", "docker")
	experimentDetails.ChaosServiceAccount = common.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	experimentDetails.SocketPath = common.Getenv("SOCKET_PATH", "/var/run/docker.sock")
	experimentDetails.Sequence = common.Getenv("SEQUENCE", "parallel")
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(common.Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))
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
	chaosDetails.Targets = []v1alpha1.TargetDetails{}
	chaosDetails.DefaultAppHealthCheck, _ = strconv.ParseBool(common.Getenv("DEFAULT_APP_HEALTH_CHECK", "true"))
}
