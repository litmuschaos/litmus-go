package environment

import (
	"encoding/json"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"

	clientTypes "k8s.io/apimachinery/pkg/types"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/spring-boot/spring-boot-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

// GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", expName)
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(types.Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	experimentDetails.ChaosLib = types.Getenv("LIB", "litmus")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	experimentDetails.TargetPods = types.Getenv("TARGET_PODS", "")
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(types.Getenv("PODS_AFFECTED_PERC", "0"))
	experimentDetails.Sequence = types.Getenv("SEQUENCE", "serial")

	// Chaos monkey assault parameters
	experimentDetails.ChaosMonkeyPath = types.Getenv("CM_PATH", "/actuator/chaosmonkey")
	experimentDetails.ChaosMonkeyPort = types.Getenv("CM_PORT", "8080")

	level, _ := strconv.Atoi(types.Getenv("CM_LEVEL", "1"))
	deterministic := true
	watchedCustomServices := strings.Split(types.Getenv("CM_WATCHED_CUSTOM_SERVICES", ""), ",")

	switch expName {
	case "spring-boot-app-kill":
		// kill application assault
		assault := experimentTypes.AppKillAssault{
			Level:                 level,
			Deterministic:         deterministic,
			WatchedCustomServices: watchedCustomServices,
			KillApplicationActive: true,
			KillApplicationCron:   "*/1 * * * * ?",
		}
		experimentDetails.ChaosMonkeyAssault, _ = json.Marshal(assault)
	case "spring-boot-latency":
		latency, _ := strconv.Atoi(types.Getenv("LATENCY", "2000"))
		// Latency assault
		assault := experimentTypes.LatencyAssault{
			Level:                 level,
			Deterministic:         deterministic,
			WatchedCustomServices: watchedCustomServices,
			LatencyActive:         true,
			LatencyRangeStart:     latency,
			LatencyRangeEnd:       latency,
		}
		experimentDetails.ChaosMonkeyAssault, _ = json.Marshal(assault)
	case "spring-boot-memory-stress":
		memoryFillPercentage, _ := strconv.ParseFloat(types.Getenv("MEMORY_FILL_FRACTION", "0.7"), 64)
		// Memory assault
		assault := experimentTypes.MemoryStressAssault{
			Level:                              level,
			Deterministic:                      deterministic,
			WatchedCustomServices:              watchedCustomServices,
			MemoryActive:                       true,
			MemoryMillisecondsHoldFilledMemory: experimentDetails.ChaosDuration * 1000,
			MemoryMillisecondsWaitNextIncrease: 1000,
			MemoryFillIncrementFraction:        1.0,
			MemoryCron:                         "*/1 * * * * ?",
			MemoryFillTargetFraction:           memoryFillPercentage,
		}
		experimentDetails.ChaosMonkeyAssault, _ = json.Marshal(assault)
	case "spring-boot-cpu-stress":
		cpuLoadTargetFraction, _ := strconv.ParseFloat(types.Getenv("CPU_LOAD_FRACTION", "0.9"), 64)
		// CPU assault
		assault := experimentTypes.CPUStressAssault{
			Level:                   level,
			Deterministic:           deterministic,
			WatchedCustomServices:   watchedCustomServices,
			CPUActive:               true,
			CPUMillisecondsHoldLoad: experimentDetails.ChaosDuration * 1000,
			CPULoadTargetFraction:   cpuLoadTargetFraction,
			CPUCron:                 "*/1 * * * * ?",
		}
		experimentDetails.ChaosMonkeyAssault, _ = json.Marshal(assault)
	case "spring-boot-exceptions":
		// Exception assault
		assault := experimentTypes.ExceptionAssault{
			Level:                 level,
			Deterministic:         deterministic,
			WatchedCustomServices: watchedCustomServices,
			ExceptionsActive:      true,
		}

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
		log.InfoWithValues("[Info]: Chaos monkeys exceptions assaults details", logrus.Fields{
			"Exception Type":      assault.Exception.Type,
			"Exception Arguments": assault.Exception.Arguments,
		})
		experimentDetails.ChaosMonkeyAssault, _ = json.Marshal(assault)
	}

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
