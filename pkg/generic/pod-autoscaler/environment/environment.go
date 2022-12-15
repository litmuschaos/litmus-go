package environment

import (
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-autoscaler/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {

	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "pod-autoscaler")
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.AppAffectPercentage, _ = strconv.Atoi(types.Getenv("APP_AFFECT_PERC", "100"))
	experimentDetails.Replicas, _ = strconv.Atoi(types.Getenv("REPLICA_COUNT", ""))
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.AuxiliaryAppInfo = types.Getenv("AUXILIARY_APPINFO", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetContainer = types.Getenv("TARGET_CONTAINER", "")

	experimentDetails.AppNS, experimentDetails.AppKind, experimentDetails.AppLabel = getAppDetails()
}

func getAppDetails() (string, string, string) {
	targets := types.Getenv("TARGETS", "")
	app := types.GetTargets(targets)
	if len(app) != 0 && (app[0].Kind == "deployment" || app[0].Kind == "statefulset") {
		return app[0].Namespace, app[0].Kind, app[0].Labels[0]
	}
	return "", "", ""
}
