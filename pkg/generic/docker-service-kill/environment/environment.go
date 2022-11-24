package environment

import (
	"strconv"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/docker-service-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "docker-service-kill")
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "90"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.AuxiliaryAppInfo = types.Getenv("AUXILIARY_APPINFO", "")
	experimentDetails.TargetNode = types.Getenv("TARGET_NODE", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.LIBImage = types.Getenv("LIB_IMAGE", "ubuntu:16.04")
	experimentDetails.LIBImagePullPolicy = types.Getenv("LIB_IMAGE_PULL_POLICY", "Always")
	experimentDetails.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	experimentDetails.NodeLabel = types.Getenv("NODE_LABEL", "")
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(types.Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))
	experimentDetails.SetHelperData = types.Getenv("SET_HELPER_DATA", "true")

	experimentDetails.AppNS, experimentDetails.AppKind, experimentDetails.AppLabel = getAppDetails()
}

func getAppDetails() (string, string, string) {
	targets := types.Getenv("TARGETS", "")
	app := types.GetTargets(targets)
	if len(app) != 0 {
		return app[0].Namespace, app[0].Kind, app[0].Labels[0]
	}
	return "", "", ""
}
