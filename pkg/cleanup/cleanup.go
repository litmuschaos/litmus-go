// Package cleanup recognises revert failures that were caused by the target
// itself no longer existing.
//
// When a target pod dies mid-experiment (evicted, rescheduled, OOMKilled, ...)
// its namespaces and processes go away with it. The injected fault dies with
// them, so there is nothing left to revert, yet the revert command still fails
// because it cannot enter a namespace that is gone. That is not a cleanup
// failure, it is the acknowledgement that the cleanup is already complete.
package cleanup

import (
	"strings"
)

// TargetTypePod is the only target type these helpers apply to. For node and
// container targets "the target is gone" does not imply "already cleaned up",
// and treating it that way could leave residual chaos in the cluster.
const TargetTypePod = "pod"

// targetGoneSignatures are the error signatures emitted when the revert could
// not run because the target's namespace or process has already disappeared.
var targetGoneSignatures = []string{
	// nsenter could not open the namespace path of a target that is gone,
	// e.g. "nsenter: cannot open /proc/4215/ns/net: No such file or directory"
	"cannot open /proc/",
	// nsenter resolved the path but the namespace was torn down meanwhile
	"setns: no such file or directory",
	// the target pid is no longer alive
	"no such process",
}

// IsTargetGone reports whether the given revert output or error message says
// that the target's namespace or process no longer exists.
func IsTargetGone(output string) bool {
	if strings.TrimSpace(output) == "" {
		return false
	}
	out := strings.ToLower(output)
	for _, signature := range targetGoneSignatures {
		if strings.Contains(out, signature) {
			return true
		}
	}
	return false
}

// IsRevertFailureTolerable reports whether a failed revert may be treated as
// complete, because the pod it was injected into is gone and took the fault
// with it. Any other revert failure is still a real failure.
//
// This only governs the cleanup phase, never the verdict: probes and
// steady-state checks continue to decide whether the experiment passed.
func IsRevertFailureTolerable(targetType, output string) bool {
	return AreRevertFailuresTolerable(targetType, []string{output})
}

// AreRevertFailuresTolerable is the multi-step form of
// IsRevertFailureTolerable, for a revert made of several independent steps.
//
// Every single failure must indicate that the target is gone. Checking the
// steps as one joined string would let a gone-target error from one step hide
// a genuine failure in another, e.g. a proxy that is already dead masking an
// iptables rule that could not be removed, which would leave the rule behind.
func AreRevertFailuresTolerable(targetType string, outputs []string) bool {
	if len(outputs) == 0 {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(targetType), TargetTypePod) {
		return false
	}
	for _, output := range outputs {
		if !IsTargetGone(output) {
			return false
		}
	}
	return true
}
