package environment

import (
	"strconv"
	"strings"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimentTypes "pkg/azure/instance-runscript/types"

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
	isBase64Str := types.Getenv("IS_BASE64", "false")
	experimentDetails.IsBase64, _ = strconv.ParseBool(isBase64Str)
	experimentDetails.PowershellChaosStartParamNames = strings.Split(types.Getenv("POWERSHELL_CHAOS_START_PARAMNAMES", ""))
	experimentDetails.PowershellChaosEndParamNames = strings.Split(types.Getenv("POWERSHELL_CHAOS_END_PARAMNAMES", ""))
	experimentDetails.PowershellChaosStartParamValues = strings.Split(types.Getenv("POWERSHELL_CHAOS_START_PARAMVALUES", ""))
	experimentDetails.PowershellChaosEndParamValues = strings.Split(types.Getenv("POWERSHELL_CHAOS_END_PARAMVALUES", ""))
	experimentDetails.PowershellChaosStartBase64OrPsFilePath = types.Getenv("POWERSHELL_CHAOS_START_SCRIPT_BASE64_STR_OR_PS_FILE_PATH", "")
	experimentDetails.PowershellChaosEndBase64OrPsFilePath = types.Getenv("POWERSHELL_CHAOS_END_SCRIPT_BASE64_STR_OR_PS_FILE_PATH", "")
	experimentDetails.ScaleSet = types.Getenv("SCALE_SET", "disable")
	experimentDetails.Sequence = types.Getenv("SEQUENCE", "serial")
}
