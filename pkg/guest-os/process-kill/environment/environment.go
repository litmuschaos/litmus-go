package environment

import (
	"strconv"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/guest-os/process-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

// STEPS TO GETENV OF YOUR CHOICE HERE
// ADDED FOR FEW MANDATORY FIELD

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(types.Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = types.Getenv("LIB", "litmus")
	experimentDetails.AppNS = types.Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = types.Getenv("APP_LABEL", "")
	experimentDetails.AppKind = types.Getenv("APP_KIND", "")
	experimentDetails.AuxiliaryAppInfo = types.Getenv("AUXILIARY_APPINFO","")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.ChaosInjectCmd = types.Getenv("CHAOS_INJECT_COMMAND", "")
	experimentDetails.ChaosKillCmd = types.Getenv("CHAOS_KILL_COMMAND", "")
	experimentDetails.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	experimentDetails.TargetPods = types.Getenv("TARGET_PODS", "")
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(types.Getenv("PODS_AFFECTED_PERC", "0"))
}
