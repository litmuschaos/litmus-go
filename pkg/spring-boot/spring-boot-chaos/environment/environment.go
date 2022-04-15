package environment

import (
	"strconv"
	"strings"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/spring-boot/spring-boot-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

// STEPS TO GETENV OF YOUR CHOICE HERE
// ADDED FOR FEW MANDATORY FIELD

// GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "spring-boot-chaos")
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(types.Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = types.Getenv("LIB", "litmus")
	experimentDetails.AppNS = types.Getenv("APP_NAMESPACE", "")
	experimentDetails.AppLabel = types.Getenv("APP_LABEL", "")
	experimentDetails.AppKind = types.Getenv("APP_KIND", "")
	experimentDetails.AuxiliaryAppInfo = types.Getenv("AUXILIARY_APPINFO", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.ChaosInjectCmd = types.Getenv("CHAOS_INJECT_COMMAND", "")
	experimentDetails.ChaosKillCmd = types.Getenv("CHAOS_KILL_COMMAND", "")
	experimentDetails.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	experimentDetails.TargetPods = types.Getenv("TARGET_PODS", "")
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(types.Getenv("PODS_AFFECTED_PERC", "0"))
	experimentDetails.Sequence = types.Getenv("SEQUENCE", "serial")

	// Chaos monkey assault parameters

	experimentDetails.ChaosMonkeyPath = types.Getenv("CM_PATH", "/actuator/chaosmonkey")
	experimentDetails.ChaosMonkeyPort = types.Getenv("CM_PORT", "8080")

	// Basic assault parameters
	assault := experimentTypes.ChaosMonkeyAssault{}
	assault.Level, _ = strconv.Atoi(types.Getenv("CM_LEVEL", "1"))
	assault.Deterministic, _ = strconv.ParseBool(types.Getenv("CM_DETERMINISTIC", "true"))
	assault.WatchedCustomServices = strings.Split(types.Getenv("CM_WATCHED_CUSTOM_SERVICES", ""), ",")

	// kill application assault
	assault.KillApplicationActive, _ = strconv.ParseBool(types.Getenv("CM_KILL_APPLICATION_ACTIVE", "false"))
	assault.KillApplicationCron = types.Getenv("CM_KILL_APPLICATION_CRON", "OFF")

	// Latency assault
	assault.LatencyActive, _ = strconv.ParseBool(types.Getenv("CM_LATENCY_ACTIVE", "false"))
	assault.LatencyRangeStart, _ = strconv.Atoi(types.Getenv("CM_LATENCY_RANGE_START", "500"))
	assault.LatencyRangeEnd, _ = strconv.Atoi(types.Getenv("CM_LATENCY_RANGE_END", "500"))

	// Memory assault
	assault.MemoryActive, _ = strconv.ParseBool(types.Getenv("CM_MEMORY_ACTIVE", "false"))
	assault.MemoryMillisecondsHoldFilledMemory, _ = strconv.Atoi(types.Getenv("CM_MEMORY_MS_HOLD_FILLED_MEM", "90000"))
	assault.MemoryMillisecondsWaitNextIncrease, _ = strconv.Atoi(types.Getenv("CM_MEMORY_MS_NEXT_INCREASE", "1000"))
	assault.MemoryFillIncrementFraction, _ = strconv.ParseFloat(types.Getenv("CM_MEMORY_FILL_INC_FRACTION", "0.15"), 64)
	assault.MemoryFillTargetFraction, _ = strconv.ParseFloat(types.Getenv("CM_MEMORY_FILL_TARGET_FRACTION", "0.25"), 64)
	assault.MemoryCron = types.Getenv("CM_MEMORY_CRON", "OFF")

	// CPU assault
	assault.CPUActive, _ = strconv.ParseBool(types.Getenv("CM_CPU_ACTIVE", "false"))
	assault.CPUMillisecondsHoldLoad, _ = strconv.Atoi(types.Getenv("CM_CPU_MS_HOLD_LOAD", "90000"))
	assault.CPULoadTargetFraction, _ = strconv.ParseFloat(types.Getenv("CM_CPU_LOAD_TARGET_FRACTION", "0.9"), 64)
	assault.CPUCron = types.Getenv("CM_CPU_CRON", "OFF")

	// Exception assault
	assault.ExceptionsActive, _ = strconv.ParseBool(types.Getenv("CM_EXCEPTIONS_ACTIVE", "false"))

	// Exception structure, will be like : {type: "", arguments: [{className: "", value: ""]}
	assaultException := experimentTypes.AssaultException{}
	assaultExceptionArguments := make([]experimentTypes.AssaultExceptionArgument, 0)

	assaultException.Type = types.Getenv("CM_EXCEPTIONS_TYPE", "")

	envAssaultExceptionArguments := strings.Split(types.Getenv("CM_EXCEPTIONS_ARGUMENTS", ""), ",")

	for _, argument := range envAssaultExceptionArguments {
		splitArgument := strings.Split(argument, ":")
		assaultExceptionArgument := experimentTypes.AssaultExceptionArgument{
			ClassName: splitArgument[0],
			Value:     "",
		}
		if len(splitArgument) > 0 {
			assaultExceptionArgument.Value = splitArgument[1]
		}
		assaultExceptionArguments = append(assaultExceptionArguments, assaultExceptionArgument)
	}
	assaultException.Arguments = assaultExceptionArguments
	assault.Exception = assaultException

	// End of assault building
	experimentDetails.ChaosMonkeyAssault = assault

	// Building watchers
	watchers := experimentTypes.ChaosMonkeyWatchers{
		Controller:     false,
		RestController: false,
		Service:        false,
		Repository:     false,
		Component:      false,
		RestTemplate:   false,
		WebClient:      false,
	}

	envWatchers := strings.Split(types.Getenv("CM_WATCHERS", ""), ",")
	for _, watcher := range envWatchers {
		switch watcher {
		case "controller":
			watchers.Controller = true
		case "restController":
			watchers.RestController = true
		case "service":
			watchers.Service = true
		case "repository":
			watchers.Repository = true
		case "component":
			watchers.Component = true
		case "webClient":
			watchers.WebClient = true
		default:
		}
	}
	experimentDetails.ChaosMonkeyWatchers = watchers
}
