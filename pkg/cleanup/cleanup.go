// Package cleanup implements the CLEANUP_POLICY behaviour, which decides how a
// fault should treat a revert that could not be performed because the target
// itself no longer exists.
//
// When a target pod dies mid-experiment (evicted, rescheduled, OOMKilled, ...)
// its namespaces and processes go away with it. The injected fault dies with
// them, so there is nothing left to revert, yet the revert command still fails
// because it cannot enter a namespace that is gone. That is not a cleanup
// failure, it is the acknowledgement that the cleanup is trivially complete.
package cleanup

import (
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

const (
	// PolicyFail marks the experiment as failed when the fault could not be
	// reverted, even if the target is gone. It is the default, so that
	// existing setups keep their current behaviour.
	PolicyFail = "fail"
	// PolicyIgnore treats the revert as complete when the failure was caused
	// by the target no longer existing.
	PolicyIgnore = "ignore"
)

// TargetTypePod is the only target type the policy applies to. For node and
// container targets "the target is gone" does not imply "already cleaned up",
// and honouring the policy there could leave residual chaos in the cluster.
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

// Policy returns the configured cleanup policy, defaulting to PolicyFail.
// An unrecognised value falls back to PolicyFail so that a typo can never
// silently weaken the verdict.
func Policy() string {
	policy := strings.ToLower(strings.TrimSpace(types.Getenv("CLEANUP_POLICY", PolicyFail)))
	switch policy {
	case PolicyFail, PolicyIgnore:
		return policy
	case "":
		return PolicyFail
	default:
		log.Warnf("[Cleanup]: unknown CLEANUP_POLICY '%s', falling back to '%s'", policy, PolicyFail)
		return PolicyFail
	}
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

// ShouldIgnoreRevertFailure reports whether a failed revert may be treated as
// complete. It requires all three conditions to hold: the policy is opted in
// to "ignore", the fault targets a pod, and the failure was caused by the
// target being gone. Any other revert failure is still a real failure.
//
// This only governs the cleanup phase, never the verdict: probes and
// steady-state checks continue to decide whether the experiment passed.
func ShouldIgnoreRevertFailure(targetType, output string) bool {
	return ShouldIgnoreRevertFailures(targetType, []string{output})
}

// ShouldIgnoreRevertFailures is the multi-step form of
// ShouldIgnoreRevertFailure, for a revert made of several independent steps.
//
// Every single failure must indicate that the target is gone. Checking the
// steps as one joined string would let a gone-target error from one step hide
// a genuine failure in another, e.g. a proxy that is already dead masking an
// iptables rule that could not be removed, which would leave the rule behind.
func ShouldIgnoreRevertFailures(targetType string, outputs []string) bool {
	if len(outputs) == 0 {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(targetType), TargetTypePod) {
		return false
	}
	if Policy() != PolicyIgnore {
		return false
	}
	for _, output := range outputs {
		if !IsTargetGone(output) {
			return false
		}
	}
	return true
}
