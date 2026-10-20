package cleanup

import (
	"testing"
)

// nsenterNetNsGone is the error reported by nsenter when the target pod died
// mid-experiment and its network namespace no longer exists.
const nsenterNetNsGone = "nsenter: cannot open /proc/4215/ns/net: No such file or directory"

func TestPolicy(t *testing.T) {
	testcases := []struct {
		name     string
		env      string
		setEnv   bool
		expected string
	}{
		{name: "defaults to fail when unset", setEnv: false, expected: PolicyFail},
		{name: "defaults to fail when empty", env: "", setEnv: true, expected: PolicyFail},
		{name: "reads fail", env: "fail", setEnv: true, expected: PolicyFail},
		{name: "reads ignore", env: "ignore", setEnv: true, expected: PolicyIgnore},
		{name: "is case insensitive", env: "Ignore", setEnv: true, expected: PolicyIgnore},
		{name: "trims whitespace", env: "  ignore  ", setEnv: true, expected: PolicyIgnore},
		{name: "falls back to fail on a typo", env: "ignoer", setEnv: true, expected: PolicyFail},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv {
				t.Setenv("CLEANUP_POLICY", tc.env)
			}
			if got := Policy(); got != tc.expected {
				t.Errorf("Policy() = %q, want %q", got, tc.expected)
			}
		})
	}
}

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

func TestShouldIgnoreRevertFailure(t *testing.T) {
	testcases := []struct {
		name       string
		policy     string
		targetType string
		output     string
		expected   bool
	}{
		{
			// acceptance criterion: CLEANUP_POLICY=ignore + pod target + gone target
			name:       "ignores a gone pod target when opted in",
			policy:     PolicyIgnore,
			targetType: TargetTypePod,
			output:     nsenterNetNsGone,
			expected:   true,
		},
		{
			// acceptance criterion: the default preserves current behaviour
			name:       "fails a gone pod target under the default policy",
			policy:     PolicyFail,
			targetType: TargetTypePod,
			output:     nsenterNetNsGone,
			expected:   false,
		},
		{
			name:       "fails a gone pod target when the policy is unset",
			policy:     "",
			targetType: TargetTypePod,
			output:     nsenterNetNsGone,
			expected:   false,
		},
		{
			// acceptance criterion: node targets are unaffected by the policy
			name:       "never ignores a node target",
			policy:     PolicyIgnore,
			targetType: "node",
			output:     nsenterNetNsGone,
			expected:   false,
		},
		{
			// acceptance criterion: container targets are unaffected
			name:       "never ignores a container target",
			policy:     PolicyIgnore,
			targetType: "container",
			output:     nsenterNetNsGone,
			expected:   false,
		},
		{
			name:       "never ignores an empty target type",
			policy:     PolicyIgnore,
			targetType: "",
			output:     nsenterNetNsGone,
			expected:   false,
		},
		{
			// the policy must not mask a revert that genuinely failed
			name:       "does not ignore a genuine revert failure on a live pod",
			policy:     PolicyIgnore,
			targetType: TargetTypePod,
			output:     "RTNETLINK answers: Operation not permitted",
			expected:   false,
		},
		{
			name:       "accepts a capitalised target type",
			policy:     PolicyIgnore,
			targetType: "Pod",
			output:     nsenterNetNsGone,
			expected:   true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLEANUP_POLICY", tc.policy)
			got := ShouldIgnoreRevertFailure(tc.targetType, tc.output)
			if got != tc.expected {
				t.Errorf("ShouldIgnoreRevertFailure(%q, %q) with CLEANUP_POLICY=%q = %v, want %v",
					tc.targetType, tc.output, tc.policy, got, tc.expected)
			}
		})
	}
}

func TestShouldIgnoreRevertFailures(t *testing.T) {
	const (
		ipRulesGone    = "failed to remove ip rules: nsenter: cannot open /proc/4215/ns/net: No such file or directory"
		proxyGone      = "failed to stop proxy server: kill: (4215): No such process"
		ipRulesRealErr = "failed to remove ip rules: iptables: Operation not permitted"
	)

	testcases := []struct {
		name       string
		policy     string
		targetType string
		outputs    []string
		expected   bool
	}{
		{
			name:       "ignores a multi step revert where every step reports a gone target",
			policy:     PolicyIgnore,
			targetType: TargetTypePod,
			outputs:    []string{ipRulesGone, proxyGone},
			expected:   true,
		},
		{
			// a gone proxy must not hide an iptables rule that could not be
			// removed, otherwise the rule is left behind on a live namespace
			name:       "does not ignore a genuine failure paired with a gone target",
			policy:     PolicyIgnore,
			targetType: TargetTypePod,
			outputs:    []string{ipRulesRealErr, proxyGone},
			expected:   false,
		},
		{
			name:       "order of the steps does not matter",
			policy:     PolicyIgnore,
			targetType: TargetTypePod,
			outputs:    []string{proxyGone, ipRulesRealErr},
			expected:   false,
		},
		{
			name:       "does not ignore when every step genuinely failed",
			policy:     PolicyIgnore,
			targetType: TargetTypePod,
			outputs:    []string{ipRulesRealErr, ipRulesRealErr},
			expected:   false,
		},
		{
			name:       "no failures is not something to ignore",
			policy:     PolicyIgnore,
			targetType: TargetTypePod,
			outputs:    nil,
			expected:   false,
		},
		{
			name:       "default policy still fails a fully gone target",
			policy:     PolicyFail,
			targetType: TargetTypePod,
			outputs:    []string{ipRulesGone, proxyGone},
			expected:   false,
		},
		{
			name:       "node targets are never ignored",
			policy:     PolicyIgnore,
			targetType: "node",
			outputs:    []string{ipRulesGone, proxyGone},
			expected:   false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLEANUP_POLICY", tc.policy)
			if got := ShouldIgnoreRevertFailures(tc.targetType, tc.outputs); got != tc.expected {
				t.Errorf("ShouldIgnoreRevertFailures(%q, %v) with CLEANUP_POLICY=%q = %v, want %v",
					tc.targetType, tc.outputs, tc.policy, got, tc.expected)
			}
		})
	}
}
