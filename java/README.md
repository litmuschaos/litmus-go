# JVM fault injector

The JVM fault injector is a tool to inject either memory or CPU consumption into a running JVM.
For more on the background see [JVM fault injection proposal](https://github.com/litmuschaos/litmus/blob/master/proposals/jvm-fault-injection.md)

This tool is intended to be called from a helper-pod running on the same node as the targeted JVM.

It accepts the following parameters
`<process ID> <main command> <arguments>`

Process ID: this is the ID of the java process.

Main command: this is either `mem` for memory consumption, `cpu` for CPU consumption or `stop` to stop any ongoing fault injection.

Arguments: available arguments depends on main command.

## mem

The mem command will fill up memory. It will run in a loop allocating memory for each iteration for a specified duration.

| Option | Description |
| --- | --- |
| -k | Keep references to the allocated memory. This will prevent the allocated memory to be garbage collected. Without this option memory can be garbage collected. |
| -d [integer] | Duration. The chaos will run for the number of seconds provided. Default 60 seconds.  |
| -m  [integer] | How much memory, in mebibytes, that will be allocated for each iteration. Default is 1 MiB. (Max possible amount is 2047) |
| -s [integer] | How many milliseconds of sleep time there will be for each iteration. Default 2000 milliseconds |

## cpu

The cpu command will generate CPU load for a period of time.

| Option | Description |
| --- | --- |
| -d [integer] | Duration in seconds. Default is 60 seconds. |
| -t [integer] | Number of concurrent threads to use. Default 1. |

## stop

The stop command will stop any ongoing fault injection.
