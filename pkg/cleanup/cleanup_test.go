package cleanup

import (
	"testing"
)

// nsenterNetNsGone is the error reported by nsenter when the target pod died
// mid-experiment and its network namespace no longer exists.
const nsenterNetNsGone = "nsenter: cannot open /proc/4215/ns/net: No such file or directory"

func TestIsTargetGone(t *testing.T) {
	testcases := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "nsenter cannot open a network namespace that is gone",
			output:   nsenterNetNsGone,
			expected: true,
		},
		{
			name:     "nsenter cannot open a pid namespace that is gone",
			output:   "nsenter: cannot open /proc/4215/ns/pid: No such file or directory",
			expected: true,
		},
		{
			name:     "setns raced with the namespace teardown",
			output:   "setns: No such file or directory",
			expected: true,
		},
		{
			name:     "the target pid is no longer alive",
			output:   "kill: (4215): No such process",
			expected: true,
		},
		{
			name:     "a genuine revert failure is not a gone target",
			output:   "RTNETLINK answers: Operation not permitted",
			expected: false,
		},
		{
			name:     "a missing qdisc is not a gone target",
			output:   "RTNETLINK answers: No such file or directory",
			expected: false,
		},
		{
			name:     "empty output",
			output:   "",
			expected: false,
		},
		{
			name:     "whitespace only output",
			output:   "   \n\t ",
			expected: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTargetGone(tc.output); got != tc.expected {
				t.Errorf("IsTargetGone(%q) = %v, want %v", tc.output, got, tc.expected)
			}
		})
	}
}

func TestIsRevertFailureTolerable(t *testing.T) {
	testcases := []struct {
		name       string
		targetType string
		output     string
		expected   bool
	}{
		{
			name:       "tolerates a gone pod target",
			targetType: TargetTypePod,
			output:     nsenterNetNsGone,
			expected:   true,
		},
		{
			// for a node, "gone" does not mean "already cleaned up"
			name:       "never tolerates a node target",
			targetType: "node",
			output:     nsenterNetNsGone,
			expected:   false,
		},
		{
			name:       "never tolerates a container target",
			targetType: "container",
			output:     nsenterNetNsGone,
			expected:   false,
		},
		{
			name:       "never tolerates an empty target type",
			targetType: "",
			output:     nsenterNetNsGone,
			expected:   false,
		},
		{
			// the tolerance must not mask a revert that genuinely failed
			name:       "does not tolerate a genuine revert failure on a live pod",
			targetType: TargetTypePod,
			output:     "RTNETLINK answers: Operation not permitted",
			expected:   false,
		},
		{
			name:       "accepts a capitalised target type",
			targetType: "Pod",
			output:     nsenterNetNsGone,
			expected:   true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRevertFailureTolerable(tc.targetType, tc.output); got != tc.expected {
				t.Errorf("IsRevertFailureTolerable(%q, %q) = %v, want %v",
					tc.targetType, tc.output, got, tc.expected)
			}
		})
	}
}

func TestAreRevertFailuresTolerable(t *testing.T) {
	const (
		ipRulesGone    = "failed to remove ip rules: nsenter: cannot open /proc/4215/ns/net: No such file or directory"
		proxyGone      = "failed to stop proxy server: kill: (4215): No such process"
		ipRulesRealErr = "failed to remove ip rules: iptables: Operation not permitted"
	)

	testcases := []struct {
		name       string
		targetType string
		outputs    []string
		expected   bool
	}{
		{
			name:       "tolerates a multi step revert where every step reports a gone target",
			targetType: TargetTypePod,
			outputs:    []string{ipRulesGone, proxyGone},
			expected:   true,
		},
		{
			// a gone proxy must not hide an iptables rule that could not be
			// removed, otherwise the rule is left behind on a live namespace
			name:       "does not tolerate a genuine failure paired with a gone target",
			targetType: TargetTypePod,
			outputs:    []string{ipRulesRealErr, proxyGone},
			expected:   false,
		},
		{
			name:       "order of the steps does not matter",
			targetType: TargetTypePod,
			outputs:    []string{proxyGone, ipRulesRealErr},
			expected:   false,
		},
		{
			name:       "does not tolerate when every step genuinely failed",
			targetType: TargetTypePod,
			outputs:    []string{ipRulesRealErr, ipRulesRealErr},
			expected:   false,
		},
		{
			name:       "no failures is not something to tolerate",
			targetType: TargetTypePod,
			outputs:    nil,
			expected:   false,
		},
		{
			name:       "node targets are never tolerated",
			targetType: "node",
			outputs:    []string{ipRulesGone, proxyGone},
			expected:   false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AreRevertFailuresTolerable(tc.targetType, tc.outputs); got != tc.expected {
				t.Errorf("AreRevertFailuresTolerable(%q, %v) = %v, want %v",
					tc.targetType, tc.outputs, got, tc.expected)
			}
		})
	}
}
