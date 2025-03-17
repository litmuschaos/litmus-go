package environment

import (
	"strconv"
	"strings"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/instance-runscript/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

// GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "azure-instance-runscript")
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(types.Getenv("CHAOS_INTERVAL", "30"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.AzureInstanceNames = strings.TrimSpace(types.Getenv("AZURE_INSTANCE_NAMES", ""))
	experimentDetails.ResourceGroup = types.Getenv("RESOURCE_GROUP", "")
	experimentDetails.IsBase64 = strconv.ParseBool(isBase64Str)
	experimentDetails.PowershellChaosStartParams = strings.Split(types.Getenv("POWERSHELL_CHAOS_START_PARAMS", ""))
	experimentDetails.PowershellChaosEndParams = strings.Split(types.Getenv("POWERSHELL_CHAOS_END_PARAMS", ""))
	experimentDetails.PowershellChaosStartBase64OrPsFilePath = types.Getenv("POWERSHELL_CHAOS_START_SCRIPT_BASE64_STR_OR_PS_FILE_PATH", "")
	experimentDetails.PowershellChaosEndBase64OrPsFilePath = types.Getenv("POWERSHELL_CHAOS_END_SCRIPT_BASE64_STR_OR_PS_FILE_PATH", "")
	experimentDetails.ScaleSet = types.Getenv("SCALE_SET", "disable")
	experimentDetails.Sequence = types.Getenv("SEQUENCE", "serial")
}
