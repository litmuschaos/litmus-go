package environment

import (
	"strconv"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/aws-ssm/aws-ssm-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

// GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(types.Getenv("CHAOS_INTERVAL", "60"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.DocumentName = types.Getenv("DOCUMENT_NAME", "LitmusChaos-AWS-SSM-Doc")
	experimentDetails.DocumentType = types.Getenv("DOCUMENT_TYPE", "Command")
	experimentDetails.DocumentFormat = types.Getenv("DOCUMENT_FORMAT", "YAML")
	experimentDetails.DocumentPath = types.Getenv("DOCUMENT_PATH", "LitmusChaos-AWS-SSM-Docs.yml")
	experimentDetails.Region = types.Getenv("REGION", "")
	experimentDetails.Cpu, _ = strconv.Atoi(types.Getenv("CPU_CORE", "0"))
	experimentDetails.NumberOfWorkers, _ = strconv.Atoi(types.Getenv("NUMBER_OF_WORKERS", "1"))
	experimentDetails.MemoryPercentage, _ = strconv.Atoi(types.Getenv("MEMORY_PERCENTAGE", "80"))
	experimentDetails.InstallDependencies = types.Getenv("INSTALL_DEPENDENCIES", "True")
	experimentDetails.Sequence = types.Getenv("SEQUENCE", "parallel")
	switch expName {
	case "aws-ssm-chaos-by-tag":
		experimentDetails.EC2InstanceTag = types.Getenv("EC2_INSTANCE_TAG", "")
		experimentDetails.InstanceAffectedPerc, _ = strconv.Atoi(types.Getenv("INSTANCE_AFFECTED_PERC", "0"))
	case "aws-ssm-chaos-by-id":
		experimentDetails.EC2InstanceID = types.Getenv("EC2_INSTANCE_ID", "")
	}
}
