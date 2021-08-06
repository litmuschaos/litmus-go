package environment

import (
	"strconv"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/run-command/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
)

// STEPS TO GETENV OF YOUR CHOICE HERE
// ADDED FOR FEW MANDATORY FIELD

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = common.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.ChaosNamespace = common.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = common.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(common.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(common.Getenv("CHAOS_INTERVAL", "30"))
	experimentDetails.RampTime, _ = strconv.Atoi(common.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = common.Getenv("LIB", "litmus")
	experimentDetails.TargetContainer = common.Getenv("TARGET_CONTAINER", "")
	experimentDetails.AppNS = common.Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = common.Getenv("APP_LABEL", "")
	experimentDetails.AppKind = common.Getenv("APP_KIND", "")
	experimentDetails.ChaosUID = clientTypes.UID(common.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = common.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = common.Getenv("POD_NAME", "")
	experimentDetails.Delay, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.AzureInstanceNames = common.Getenv("AZURE_INSTANCE_NAME", "akash-run-command")
	experimentDetails.ResourceGroup = common.Getenv("RESOURCE_GROUP", "akash-litmus-test")
	experimentDetails.SubscriptionID = common.Getenv("SUBSCRIPTION_ID", "")
	experimentDetails.IsScaleSet = common.Getenv("IS_SCALE_SET", "false")
	experimentDetails.Sequence = common.Getenv("SEQUENCE", "parallel")
	experimentDetails.ExperimentType = common.Getenv("EXPERIMENT_TYPE", "cpu-hog")
	experimentDetails.InstallDependency = common.Getenv("INSTALL_DEPENDENCY", "True")
	experimentDetails.OperatingSystem = common.Getenv("OPERATING_SYSTEM", "linux")
	experimentDetails.CPUcores, _ = strconv.Atoi(common.Getenv("CPU_CORES", "1"))
	experimentDetails.NumberOfWorkers, _ = strconv.Atoi(common.Getenv("NUMBER_OF_WORKERS", "1"))
	experimentDetails.MemoryConsumption, _ = strconv.Atoi(common.Getenv("MEMORY_CONSUMPTION", "500"))
	experimentDetails.FilesystemUtilizationBytes, _ = strconv.Atoi(common.Getenv("FILESYSTEM_UTILIZATION_BYTES", ""))
	experimentDetails.FilesystemUtilizationPercentage, _ = strconv.Atoi(common.Getenv("FILESYSTEM_UTILIZATION_PERCENTAGE", "10"))
	experimentDetails.VolumeMountPath = common.Getenv("VOLUMNE_MOUNT_PATH", "")

}

//InitialiseChaosVariables initialise all the global variables
func InitialiseChaosVariables(chaosDetails *types.ChaosDetails, experimentDetails *experimentTypes.ExperimentDetails) {
	appDetails := types.AppDetails{}
	appDetails.AnnotationCheck, _ = strconv.ParseBool(common.Getenv("ANNOTATION_CHECK", "false"))
	appDetails.AnnotationKey = common.Getenv("ANNOTATION_KEY", "litmuschaos.io/chaos")
	appDetails.AnnotationValue = "true"
	appDetails.Kind = experimentDetails.AppKind
	appDetails.Label = experimentDetails.AppLabel
	appDetails.Namespace = experimentDetails.AppNS

	chaosDetails.ChaosNamespace = experimentDetails.ChaosNamespace
	chaosDetails.ChaosPodName = experimentDetails.ChaosPodName
	chaosDetails.ChaosUID = experimentDetails.ChaosUID
	chaosDetails.EngineName = experimentDetails.EngineName
	chaosDetails.ExperimentName = experimentDetails.ExperimentName
	chaosDetails.InstanceID = experimentDetails.InstanceID
	chaosDetails.Timeout = experimentDetails.Timeout
	chaosDetails.Delay = experimentDetails.Delay
	chaosDetails.AppDetail = appDetails
	chaosDetails.JobCleanupPolicy = common.Getenv("JOB_CLEANUP_POLICY", "retain")
	chaosDetails.ProbeImagePullPolicy = experimentDetails.LIBImagePullPolicy
}
