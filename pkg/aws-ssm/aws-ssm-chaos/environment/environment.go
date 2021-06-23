package environment

import (
	"strconv"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/aws-ssm/aws-ssm-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = common.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.ChaosNamespace = common.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = common.Getenv("CHAOSENGINE", "")
	experimentDetails.AppNS = common.Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = common.Getenv("APP_LABEL", "")
	experimentDetails.AppKind = common.Getenv("APP_KIND", "")
	experimentDetails.AuxiliaryAppInfo = common.Getenv("AUXILIARY_APPINFO", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(common.Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(common.Getenv("CHAOS_INTERVAL", "60"))
	experimentDetails.RampTime, _ = strconv.Atoi(common.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = common.Getenv("LIB", "litmus")
	experimentDetails.ChaosUID = clientTypes.UID(common.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = common.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = common.Getenv("POD_NAME", "")
	experimentDetails.Delay, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.DocumentName = common.Getenv("DOCUMENT_NAME", "LitmusChaos-AWS-SSM-Doc")
	experimentDetails.DocumentType = common.Getenv("DOCUMENT_TYPE", "Command")
	experimentDetails.DocumentFormat = common.Getenv("DOCUMENT_FORMAT", "YAML")
	experimentDetails.DocumentPath = common.Getenv("DOCUMENT_PATH", "LitmusChaos-AWS-SSM-Docs.yml")
	experimentDetails.Region = common.Getenv("REGION", "")
	experimentDetails.Cpu, _ = strconv.Atoi(common.Getenv("CPU_CORE", "0"))
	experimentDetails.NumberOfWorkers, _ = strconv.Atoi(common.Getenv("NUMBER_OF_WORKERS", "1"))
	experimentDetails.MemoryPercentage, _ = strconv.Atoi(common.Getenv("MEMORY_PERCENTAGE", "80"))
	experimentDetails.InstallDependencies = common.Getenv("INSTALL_DEPENDENCIES", "True")
	experimentDetails.Sequence = common.Getenv("SEQUENCE", "parallel")
	experimentDetails.TargetContainer = common.Getenv("TARGET_CONTAINER", "")
	switch expName {
	case "aws-ssm-chaos-by-tag":
		experimentDetails.EC2InstanceTag = common.Getenv("EC2_INSTANCE_TAG", "")
		experimentDetails.InstanceAffectedPerc, _ = strconv.Atoi(common.Getenv("INSTANCE_AFFECTED_PERC", "0"))
	case "aws-ssm-chaos-by-id":
		experimentDetails.EC2InstanceID = common.Getenv("EC2_INSTANCE_ID", "")
	}
}

//InitialiseChaosVariables initialise all the global variables
func InitialiseChaosVariables(chaosDetails *types.ChaosDetails, experimentDetails *experimentTypes.ExperimentDetails) {

	chaosDetails.ChaosNamespace = experimentDetails.ChaosNamespace
	chaosDetails.ChaosPodName = experimentDetails.ChaosPodName
	chaosDetails.ChaosUID = experimentDetails.ChaosUID
	chaosDetails.EngineName = experimentDetails.EngineName
	chaosDetails.ExperimentName = experimentDetails.ExperimentName
	chaosDetails.InstanceID = experimentDetails.InstanceID
	chaosDetails.Timeout = experimentDetails.Timeout
	chaosDetails.Delay = experimentDetails.Delay
	chaosDetails.ProbeImagePullPolicy = experimentDetails.LIBImagePullPolicy
}
