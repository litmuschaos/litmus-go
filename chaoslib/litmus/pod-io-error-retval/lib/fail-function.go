package lib

import (
	"fmt"
	"os"
	"strconv"
	"syscall"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-memory-hog/types"
)

// FailFunctionArgs is a data struct to account for the various
// parameters required for writing to /sys/kernel/debug/fail_function
type FailFunctionArgs struct {
	FuncName    string // name of the function to be injected
	RetVal      int    // value to be returned
	Probability int    // fail probability [0-100]
	Interval    int    // interval between two failures, if interval > 1,
	// we recommend setting fail probability to 100
}

// FailFunctionScript returns the sequence of commands required to inject
// fail_function failure.
func FailFunctionScript(args FailFunctionArgs) Script {
	pathPrefix := "/sys/kernel/debug/fail_function"

	return []Command{
		Command(fmt.Sprintf("echo %s > %s/inject", args.FuncName, pathPrefix)),
		Command(fmt.Sprintf("echo %d > %s/%s/retval", args.RetVal, pathPrefix, args.FuncName)),
		Command(fmt.Sprintf("echo N  > %s/task-filter", pathPrefix)),
		Command(fmt.Sprintf("echo %d > %s/probability", args.Probability, pathPrefix)),
		Command(fmt.Sprintf("echo %d > %s/interval", args.Interval, pathPrefix)),
		Command(fmt.Sprintf("echo -1 > %s/times", pathPrefix)),
		Command(fmt.Sprintf("echo 0  > %s/space", pathPrefix)),
		Command(fmt.Sprintf("echo 1  > %s/verbose", pathPrefix)),
	}
}

// ResetFailFunctionScript return the sequence of commands for reseting failure injection.
func ResetFailFunctionScript() Script {
	return []Command{Command("echo > /sys/kernel/debug/fail_function/inject")}
}

// InjectFailFunction injects a fail function failure with the given executor and the given
// fail_function arguments. It writes errors if any, or nil in an error channel passed to
// this function.
func InjectFailFunction(executor Executor, failFunctionArgs interface{}, errChannel chan error) {
	errChannel <- FailFunctionScript(failFunctionArgs.(FailFunctionArgs)).
		RunOn(Shell{executor: executor, err: nil})
}

func ResetFailFunction(executor Executor, _ interface{}) error {
	return ResetFailFunctionScript().RunOn(Shell{executor: executor, err: nil})
}

func safeGetEnv(key string, defaultValue string) string {
	value := os.Getenv(key)

	if value == "" {
		value = defaultValue
	}

	return value
}

func safeAtoi(s string, defaultValue int) int {
	i, err := strconv.Atoi(s)

	if err != nil {
		i = defaultValue
	}

	return i
}

// FailFunctionParamsFn provides parameters for the FailFunction script with some default values.
// By default: these values emulate that 50% of read() system calls will return EIO.
func FailFunctionParamsFn(_ *experimentTypes.ExperimentDetails) interface{} {
	return FailFunctionArgs{
		FuncName:    safeGetEnv("FAIL_FUNCTION_FUNC_NAME", "read"),
		RetVal:      safeAtoi(os.Getenv("FAIL_FUNCTION_RETVAL"), int(syscall.EIO)),
		Probability: safeAtoi(os.Getenv("FAIL_FUNCTION_PROBABILITY"), 50),
		Interval:    safeAtoi(os.Getenv("FAIL_FUNCTION_INTERVAL"), 0),
	}
}

func FailFunctionLitmusChaosInjector() LitmusChaosInjector {
	return LitmusChaosInjector{
		ChaosParamsFn:   FailFunctionParamsFn,
		ChaosInjectorFn: InjectFailFunction,
		ResetChaosFn:    ResetFailFunction,
	}
}

func OrchestrateFailFunctionExperiment(exp ExperimentOrchestrationDetails) error {
	return OrchestrateExperiment(exp, FailFunctionLitmusChaosInjector())
}
