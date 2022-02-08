package environment

import (
	"strconv"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/chaos-toolkit/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")

	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.Sequence = types.Getenv("SEQUENCE", "serial")

	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "20"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(types.Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))

	experimentDetails.ChaosLib = types.Getenv("LIB", "litmus")
	experimentDetails.ChaosServiceAccount = types.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	experimentDetails.LIBImagePullPolicy = types.Getenv("LIB_IMAGE_PULL_POLICY", "Always")
	experimentDetails.LIBImage = types.Getenv("LIB_IMAGE", "litmuschaos/go-runner:latest")

	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(types.Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))

	experimentDetails.AppName = types.Getenv("APP_NAME", "")
	experimentDetails.OrgName = types.Getenv("ORG_NAME", "")
	experimentDetails.SpaceName = types.Getenv("SPACE_NAME", "")

	experimentDetails.CredentialsSecretName = types.Getenv("SECRET_NAME", "cloud-foundry-credentials")

}
