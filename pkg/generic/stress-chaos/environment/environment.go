package environment

import (
	"strconv"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/stress-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = types.Getenv("LIB", "litmus")
	experimentDetails.AppNS = types.Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = types.Getenv("APP_LABEL", "")
	experimentDetails.AppKind = types.Getenv("APP_KIND", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.LIBImage = types.Getenv("LIB_IMAGE", "litmuschaos/go-runner:latest")
	experimentDetails.LIBImagePullPolicy = types.Getenv("LIB_IMAGE_PULL_POLICY", "Always")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetPods = types.Getenv("TARGET_PODS", "")
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(types.Getenv("PODS_AFFECTED_PERC", "0"))
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "docker")
	experimentDetails.ChaosServiceAccount = types.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "/var/run/docker.sock")
	experimentDetails.Sequence = types.Getenv("SEQUENCE", "parallel")
	experimentDetails.TerminationGracePeriodSeconds, _ = strconv.Atoi(types.Getenv("TERMINATION_GRACE_PERIOD_SECONDS", ""))
	experimentDetails.StressImage = types.Getenv("STRESS_IMAGE", "alexeiled/stress-ng:latest-ubuntu")

	switch expName {
	case "pod-cpu-hog":
		experimentDetails.CPUcores, _ = strconv.Atoi(types.Getenv("CPU_CORES", "1"))
		experimentDetails.StressType = "pod-cpu-stress"

	case "pod-memory-hog":
		experimentDetails.MemoryConsumption, _ = strconv.Atoi(types.Getenv("MEMORY_CONSUMPTION", "500"))
		experimentDetails.NumberOfWorkers, _ = strconv.Atoi(types.Getenv("NUMBER_OF_WORKERS", "4"))
		experimentDetails.StressType = "pod-memory-stress"

	case "pod-io-stress":
		experimentDetails.FilesystemUtilizationPercentage, _ = strconv.Atoi(types.Getenv("FILESYSTEM_UTILIZATION_PERCENTAGE", ""))
		experimentDetails.FilesystemUtilizationBytes, _ = strconv.Atoi(types.Getenv("FILESYSTEM_UTILIZATION_BYTES", ""))
		experimentDetails.NumberOfWorkers, _ = strconv.Atoi(types.Getenv("NUMBER_OF_WORKERS", "4"))
		experimentDetails.VolumeMountPath = types.Getenv("VOLUME_MOUNT_PATH", "")
		experimentDetails.CPUcores, _ = strconv.Atoi(types.Getenv("CPU_CORES", "0"))
		experimentDetails.StressType = "pod-io-stress"
	}
}
