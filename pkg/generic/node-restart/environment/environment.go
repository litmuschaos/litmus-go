package environment

import (
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-restart/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "node-restart")
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = types.Getenv("LIB", "litmus")
	experimentDetails.AppNS = types.Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = types.Getenv("APP_LABEL", "")
	experimentDetails.AppKind = types.Getenv("APP_KIND", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.LIBImage = types.Getenv("LIB_IMAGE", "litmuschaos/go-runner:latest")
	experimentDetails.LIBImagePullPolicy = types.Getenv("LIB_IMAGE_PULL_POLICY", "Always")
	experimentDetails.AuxiliaryAppInfo = types.Getenv("AUXILIARY_APPINFO", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.SSHUser = types.Getenv("SSH_USER", "root")
	experimentDetails.RebootCommand = types.Getenv("REBOOT_COMMAND", "sudo systemctl reboot")
	experimentDetails.TargetNode = types.Getenv("TARGET_NODE", "")
	experimentDetails.TargetNodeIP = types.Getenv("TARGET_NODE_IP", "")
	experimentDetails.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	experimentDetails.NodeLabel = types.Getenv("NODE_LABEL", "")
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(types.Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))
}
