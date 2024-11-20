package environment

import (
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/load/k6-loadgen/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

// GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.LIBImagePullPolicy = types.Getenv("LIB_IMAGE_PULL_POLICY", "Always")
	experimentDetails.LIBImage = types.Getenv("LIB_IMAGE", "ghcr.io/grafana/k6-operator:latest-runner")
	experimentDetails.ScriptSecretName = types.Getenv("SCRIPT_SECRET_NAME", "k6-script")
	experimentDetails.ScriptSecretKey = types.Getenv("SCRIPT_SECRET_KEY", "script.js")

}
