# Implementing IO chaos, incorporating hexagonal design, improving code reuse and more.

This is a document intended to outline my work on implementing IO chaos in litmus-go. It is still a work in progress.
Along the way, I found oppurtunities for further improvement in existing work, in addition to some problems.
I shall attempt to briefly elucidate everything here.

## Approach for injecting chaos

There are two ways to inject IO chaos in a linux application
- Injecting failures with debugfs fail_function capability on IO system calls like 
read(), open() etc.
- Mounting a block device with memory holes as a volume.

This document explains the first approach.

### Chaos injection process in detail
As explained we use debugfs fail_function capability in the linux kernel. The following
pre-requisites are necessary to use it:
- The nodes on with the pods are run must run a linux kernel compiled with
`CONFIG_FAIL_FUNCTION` flag enabled. https://github.com/torvalds/linux/blob/master/lib/Kconfig.debug#L1896

- /sys/kernel/debug has to be mounted as a debugfs file system in the containers.
See here: https://github.com/weaveworks/scope/issues/2784#issuecomment-320116047

From the linux kernel docs https://www.kernel.org/doc/html/latest/fault-injection/fault-injection.html#debugfs-entries:

>## Configure fault-injection capabilities behavior
>### debugfs entries
> ...
>
>- /sys/kernel/debug/fail_function/inject:
>
>    Format: { ‘function-name’ | ‘!function-name’ | ‘’ }
>
>    specifies the target function of error injection by name. If the function name leads ‘!’ prefix, given function is removed from injection list. If nothing specified (‘’) injection list is cleared.
>
>- /sys/kernel/debug/fail_function/injectable:
>
>    (read only) shows error injectable functions and what type of error values can be specified. The error type will be one of below; - NULL: retval must be 0. - ERRNO: retval must be -1 to -MAX_ERRNO (-4096). - ERR_NULL: retval must be 0 or -1 to -MAX_ERRNO (-4096).
>
>- /sys/kernel/debug/fail_function/{function-name}/retval:
>
>    specifies the “error” return value to inject to the given function for given function. This will be created when user specifies new injection entry.

We follow the following script from the linux kernel docs as reference for using fail_function debugfs capability:

>- Inject open_ctree error while btrfs mount:
>```
>#!/bin/bash
>
>rm -f testfile.img
>dd if=/dev/zero of=testfile.img bs=1M seek=1000 count=1
>DEVICE=$(losetup --show -f testfile.img)
>mkfs.btrfs -f $DEVICE
>mkdir -p tmpmnt
>
>FAILTYPE=fail_function
>FAILFUNC=open_ctree
>echo $FAILFUNC > /sys/kernel/debug/$FAILTYPE/inject
>echo -12 > /sys/kernel/debug/$FAILTYPE/$FAILFUNC/retval
>echo N > /sys/kernel/debug/$FAILTYPE/task-filter
>echo 100 > /sys/kernel/debug/$FAILTYPE/probability
>echo 0 > /sys/kernel/debug/$FAILTYPE/interval
>echo -1 > /sys/kernel/debug/$FAILTYPE/times
>echo 0 > /sys/kernel/debug/$FAILTYPE/space
>echo 1 > /sys/kernel/debug/$FAILTYPE/verbose
>
>mount -t btrfs $DEVICE tmpmnt
>if [ $? -ne 0 ]
>then
>    echo "SUCCESS!"
>else
>    echo "FAILED!"
>    umount tmpmnt
>fi
>
>echo > /sys/kernel/debug/$FAILTYPE/inject
>
>rmdir tmpmnt
>losetup -d $DEVICE
>rm testfile.img
>```

## Design and Implementation details

One primary aspect of injecting chaos in every experiment is executing some commands using the
`/bin/sh` shell in the target container. We need convenient abstractions for commands, shells
and scripts, for conducting experiments.

Let's follow a bottom up approach.

### Shell and the execution environment
The first abstraction is a command. This suffices:
```golang
// Command is a type for representing executable commands on a shell
type Command string
```

Then what is a script? As simple as that:
```golang
// Script represents a sequence of commands
type Script []Command
```

Now we need to abstract the behaviour of executing commands. This requires an interface:
```golang
// Executor is an interface to represent exec() providers.
type Executor interface {
	Execute(command Command) error
}
```

There can be multiple implementations of executor for different environents.

Next we have a shell:

```golang
type Shell struct {
	executor Executor
	...
}

// uses shell.executor for executing command
func (shell *Shell) Run(command Command) { ... }
```

Finally, we provide a method for running scripts on a shell:
```golang
// RunOn runs the script on the given shell
func (script Script) RunOn(shell Shell) error { ... }
```

This method runs all the commands in the script consecutively, using the shells executor, performing error handling as
necessary. Different implementations of `Executor` are passed to the `Shell` parameter.
This way the same script can run on different shells, i.e in different execution
environments.

Checkout https://github.com/arindas/litmus-go/blob/experiment/io-error/chaoslib/litmus/pod-io-error-retval/lib/shell.go
for the abstractions mentioned above.

The following implementation of the `Executor` interface is used for running commands in target containers:
(From https://github.com/arindas/litmus-go/blob/experiment/io-error/chaoslib/litmus/pod-io-error-retval/lib/litmus-executor.go#L10)
```golang
// LitmusExecutor implements the Executor interface to executing commands on pods
type LitmusExecutor struct {
	ContainerName string
	PodName       string
	Namespace     string
	Clients       clients.ClientSets
}

// Execute executes commands on the current container in the current pod
func (executor LitmusExecutor) Execute(command Command) error {
	log.Infof("Executing command: $%s", command)

	execCommandDetails := litmusexec.PodDetails{}
	cmd := []string{"/bin/sh", "-c", string(command)}

	litmusexec.SetExecCommandAttributes(
		&execCommandDetails,
		executor.PodName,
		executor.ContainerName,
		executor.Namespace,
	)

	_, err := litmusexec.Exec(
		&execCommandDetails, executor.Clients, cmd)

	return err
}
```

Now this code can be reused everytime instead of having to invoke `litmusexec.Exec(...)` manually everytime. The only thing
that changes for different experiments is the `Script` thats is `RunOn` the `Shell` with this `Executor`.

### Fail function script

We represent the the various parameters required for the fail_function debugfs injection as a struct:
```golang
// FailFunctionArgs is a data struct to account for the various
// parameters required for writing to /sys/kernel/debug/fail_function
type FailFunctionArgs struct {
	FuncName    string // name of the function to be injected
	RetVal      int    // value to be returned
	Probability int    // fail probability [0-100]
	Interval    int    // interval between two failures, if interval > 1,
	// we recommend setting fail probability to 100
}
```

We introduce a function for returning a `Script` containing the commands for invoking debugfs fail_function capability.
It faithfully imitates the script from the linux kernel docs.
```golang
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
```

Now, for resetting the failure injection, we have:
```golang
// ResetFailFunctionScript return the sequence of commands for reseting failure injection.
func ResetFailFunctionScript() Script {
	return []Command{Command("echo > /sys/kernel/debug/fail_function/inject")}
}
```

If we were to implement memory hog chaos experiment, something like this would suffice:
```golang
func MemoryHogScript(memoryConsumption string) Script {
    return []Command{Command("dd if=/dev/zero of=/dev/null bs=%sM", memoryConsumption)}
}
```

__Everything else would remain the same.__ The following sections should be able to further demonstrate the degree of code reuse achievable.

### Experiment orchestration

Currently all of the Experiment*() functions in different library packages differ only in the invocation of the InjectChaos*() functions.
Hence we introduce the following abstractions:

- A struct for representing the various details required for orchestrating an experiment.
```golang
// ExperimentOrchestrationDetails bundles together all details connected to the orchestration
// of and experiment.
type ExperimentOrchestrationDetails struct {
	ExperimentDetails *experimentTypes.ExperimentDetails
	Clients           clients.ClientSets
	ResultDetails     *types.ResultDetails
	EventDetails      *types.EventDetails
	ChaosDetails      *types.ChaosDetails
	TargetPodList     corev1.PodList
}
```

- An interface for abstracting the behaviour of different chaos injection mechanisms.
```golang
// ChaosInjector is and interface for abstracting all chaos injection mechanisms
type ChaosInjector interface {
	InjectChaosInSerialMode(
		exp ExperimentOrchestrationDetails) error
	InjectChaosInParallelMode(
		exp ExperimentOrchestrationDetails) error
}
```

- And finally, a method for orchestrating experiments:
```golang
// OrchestrateExperiment orchestrates a new chaos experiment with the given experiment details
// and the ChaosInjector for the chaos injection mechanism.
func OrchestrateExperiment(exp ExperimentOrchestrationDetails, chaosInjector ChaosInjector) error 
```

Now, in golang, it's fairly common to come across code like this:
```golang

if err := task1(); err != nil {
    return err
}

if err := task2(): err != nil {
    return err
}
...
```

This unnecessarily convolutes golang code to the point where it is difficult to graps the underlying
task being accomplished with the code. In order to deal with this we use a pattern called _Go Lift_.

Read more about this here: https://about.sourcegraph.com/go/go-lift/

Tl;dr we wrap errors in structs and use them to safely invoke methods. For instance in the above case:

```golang
type struct safeTask {
    err error
}

func (s *safeTask) task1() {
    if s.err != nil {
        return
    }

    s.err = task1()
}

func (s *safeTask) task2() {
    if s.err != nil {
        return
    }

    s.err = task2()
}

...

// In application code
s := safeTask{}
s.task1()
s.task2()
```

At first glance, it might seem more effort and LOC. However, remember that 80% of the developers only work with the
application code. If this approach is used, any developer is able to grasp what's happening from the application
code (which in our case is the sequential invocation of `task1` and `task2`).

We use the same pattern in `OrchestrateExperiment`. As a result:
```golang
// OrchestrateExperiment orchestrates a new chaos experiment with the given experiment details
// and the ChaosInjector for the chaos injection mechanism.
func OrchestrateExperiment(exp ExperimentOrchestrationDetails, chaosInjector ChaosInjector) error {
	safeExperimentOrchestrator := safeExperiment{experiment: exp}

	safeExperimentOrchestrator.verifyAppLabelOrTargetPodSpecified()
	safeExperimentOrchestrator.obtainTargetPods()
	safeExperimentOrchestrator.logTargetPodNames()
	safeExperimentOrchestrator.obtainTargetContainer()
	safeExperimentOrchestrator.injectChaos(chaosInjector)

	return safeExperimentOrchestrator.err
}
```

Compare this with other Experiment*() functions on this repository. You are welcome.