package common

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/types"
	corev1 "k8s.io/api/core/v1"
)

func TestGetStatusMessage(t *testing.T) {
	tests := []struct {
		name         string
		defaultCheck bool
		defaultMsg   string
		probeStatus  string
		expected     string
	}{
		{
			name:         "default check with no probe status",
			defaultCheck: true,
			defaultMsg:   "AUT is healthy",
			probeStatus:  "",
			expected:     "AUT is healthy",
		},
		{
			name:         "default check with probe status",
			defaultCheck: true,
			defaultMsg:   "AUT is healthy",
			probeStatus:  "probe1:Passed",
			expected:     "AUT is healthy, Probes: probe1:Passed",
		},
		{
			name:         "skip default check with no probe status",
			defaultCheck: false,
			defaultMsg:   "AUT is healthy",
			probeStatus:  "",
			expected:     "Skipped the default checks",
		},
		{
			name:         "skip default check with probe status",
			defaultCheck: false,
			defaultMsg:   "AUT is healthy",
			probeStatus:  "probe1:Passed,probe2:Passed",
			expected:     "Probes: probe1:Passed,probe2:Passed",
		},
		{
			name:         "default check with empty default message and probe status",
			defaultCheck: true,
			defaultMsg:   "",
			probeStatus:  "probe1:Passed",
			expected:     ", Probes: probe1:Passed",
		},
		{
			name:         "default check with empty default message and no probe status",
			defaultCheck: true,
			defaultMsg:   "",
			probeStatus:  "",
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetStatusMessage(tt.defaultCheck, tt.defaultMsg, tt.probeStatus)
			if got != tt.expected {
				t.Errorf("GetStatusMessage() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGetRandomSequence(t *testing.T) {
	tests := []struct {
		name             string
		sequence         string
		expectOneOf      []string
		expectExactMatch bool
	}{
		{
			name:        "random lowercase returns serial or parallel",
			sequence:    "random",
			expectOneOf: []string{"serial", "parallel"},
		},
		{
			name:        "RANDOM uppercase returns serial or parallel",
			sequence:    "RANDOM",
			expectOneOf: []string{"serial", "parallel"},
		},
		{
			name:        "Random mixed case returns serial or parallel",
			sequence:    "Random",
			expectOneOf: []string{"serial", "parallel"},
		},
		{
			name:             "serial stays as serial",
			sequence:         "serial",
			expectOneOf:      []string{"serial"},
			expectExactMatch: true,
		},
		{
			name:             "parallel stays as parallel",
			sequence:         "parallel",
			expectOneOf:      []string{"parallel"},
			expectExactMatch: true,
		},
		{
			name:             "custom value stays as-is",
			sequence:         "custom-sequence",
			expectOneOf:      []string{"custom-sequence"},
			expectExactMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRandomSequence(tt.sequence)
			found := false
			for _, v := range tt.expectOneOf {
				if got == v {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("GetRandomSequence(%q) = %q, want one of %v", tt.sequence, got, tt.expectOneOf)
			}
		})
	}
}

func TestSubStringExistsInSlice(t *testing.T) {
	// SubStringExistsInSlice checks whether val contains any element from slice
	// as a substring (i.e. strings.Contains(val, v) for each v in slice).
	tests := []struct {
		name     string
		val      string
		slice    []string
		expected bool
	}{
		{
			name:     "slice element is substring of val",
			val:      "app-pod-1-abc",
			slice:    []string{"pod-1", "pod-2"},
			expected: true,
		},
		{
			name:     "no slice element is a substring of val",
			val:      "app-pod-1-abc",
			slice:    []string{"pod-99", "pod-100"},
			expected: false,
		},
		{
			name:     "exact match returns true",
			val:      "pod-1",
			slice:    []string{"pod-1", "pod-2"},
			expected: true,
		},
		{
			// strings.Contains("", "pod-1") == false: an empty val cannot contain
			// a non-empty slice element as a substring.
			name:     "empty val cannot contain non-empty slice element",
			val:      "",
			slice:    []string{"pod-1"},
			expected: false,
		},
		{
			name:     "val contains empty-string slice element",
			val:      "pod-1",
			slice:    []string{""},
			expected: true,
		},
		{
			name:     "empty slice returns false",
			val:      "pod-1",
			slice:    []string{},
			expected: false,
		},
		{
			name:     "nil slice returns false",
			val:      "pod-1",
			slice:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SubStringExistsInSlice(tt.val, tt.slice)
			if got != tt.expected {
				t.Errorf("SubStringExistsInSlice(%q, %v) = %v, want %v", tt.val, tt.slice, got, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		val      interface{}
		slice    interface{}
		expected bool
	}{
		{
			name:     "string found in slice",
			val:      "pod-1",
			slice:    []string{"pod-1", "pod-2", "pod-3"},
			expected: true,
		},
		{
			name:     "string not found in slice",
			val:      "pod-99",
			slice:    []string{"pod-1", "pod-2", "pod-3"},
			expected: false,
		},
		{
			name:     "int found in slice",
			val:      2,
			slice:    []int{1, 2, 3},
			expected: true,
		},
		{
			name:     "int not found in slice",
			val:      99,
			slice:    []int{1, 2, 3},
			expected: false,
		},
		{
			name:     "empty slice returns false",
			val:      "pod-1",
			slice:    []string{},
			expected: false,
		},
		{
			name:     "nil slice returns false",
			val:      "pod-1",
			slice:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Contains(tt.val, tt.slice)
			if got != tt.expected {
				t.Errorf("Contains(%v, %v) = %v, want %v", tt.val, tt.slice, got, tt.expected)
			}
		})
	}
}

func TestGetContainerNames(t *testing.T) {
	tests := []struct {
		name         string
		chaosDetails types.ChaosDetails
		expected     []string
	}{
		{
			name: "no sidecars returns only experiment name",
			chaosDetails: types.ChaosDetails{
				ExperimentName: "pod-delete",
				SideCar:        []types.SideCar{},
			},
			expected: []string{"pod-delete"},
		},
		{
			name: "one sidecar appended after experiment name",
			chaosDetails: types.ChaosDetails{
				ExperimentName: "pod-delete",
				SideCar: []types.SideCar{
					{Name: "helper-sidecar"},
				},
			},
			expected: []string{"pod-delete", "helper-sidecar"},
		},
		{
			name: "multiple sidecars all appended",
			chaosDetails: types.ChaosDetails{
				ExperimentName: "cpu-hog",
				SideCar: []types.SideCar{
					{Name: "sidecar-1"},
					{Name: "sidecar-2"},
				},
			},
			expected: []string{"cpu-hog", "sidecar-1", "sidecar-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetContainerNames(&tt.chaosDetails)
			if len(got) != len(tt.expected) {
				t.Errorf("GetContainerNames() len=%d, want %d; got %v", len(got), len(tt.expected), got)
				return
			}
			for i, name := range got {
				if name != tt.expected[i] {
					t.Errorf("GetContainerNames()[%d] = %q, want %q", i, name, tt.expected[i])
				}
			}
		})
	}
}

func TestBuildSidecar(t *testing.T) {
	tests := []struct {
		name         string
		chaosDetails types.ChaosDetails
		wantLen      int
		wantNames    []string
		wantMountLen int
	}{
		{
			name: "no sidecars returns empty slice",
			chaosDetails: types.ChaosDetails{
				SideCar: []types.SideCar{},
			},
			wantLen: 0,
		},
		{
			name: "single sidecar without secrets",
			chaosDetails: types.ChaosDetails{
				SideCar: []types.SideCar{
					{
						Name:            "helper",
						Image:           "busybox:latest",
						ImagePullPolicy: corev1.PullAlways,
					},
				},
			},
			wantLen:      1,
			wantNames:    []string{"helper"},
			wantMountLen: 0,
		},
		{
			name: "sidecar with secrets creates volume mounts",
			chaosDetails: types.ChaosDetails{
				SideCar: []types.SideCar{
					{
						Name:  "helper",
						Image: "busybox:latest",
						Secrets: []v1alpha1.Secret{
							{Name: "secret-1", MountPath: "/etc/secrets"},
						},
					},
				},
			},
			wantLen:      1,
			wantNames:    []string{"helper"},
			wantMountLen: 1,
		},
		{
			name: "multiple sidecars",
			chaosDetails: types.ChaosDetails{
				SideCar: []types.SideCar{
					{Name: "sidecar-1", Image: "image-1:latest"},
					{Name: "sidecar-2", Image: "image-2:latest"},
				},
			},
			wantLen:   2,
			wantNames: []string{"sidecar-1", "sidecar-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSidecar(&tt.chaosDetails)
			if len(got) != tt.wantLen {
				t.Errorf("BuildSidecar() len=%d, want %d", len(got), tt.wantLen)
				return
			}
			for i, c := range got {
				if i < len(tt.wantNames) && c.Name != tt.wantNames[i] {
					t.Errorf("BuildSidecar()[%d].Name = %q, want %q", i, c.Name, tt.wantNames[i])
				}
			}
			if tt.wantMountLen > 0 && len(got) > 0 {
				if len(got[0].VolumeMounts) != tt.wantMountLen {
					t.Errorf("BuildSidecar()[0].VolumeMounts len=%d, want %d", len(got[0].VolumeMounts), tt.wantMountLen)
				}
			}
		})
	}
}

func TestGetSidecarVolumes(t *testing.T) {
	tests := []struct {
		name         string
		chaosDetails types.ChaosDetails
		wantLen      int
		wantNames    []string
	}{
		{
			name: "no sidecars returns empty volumes",
			chaosDetails: types.ChaosDetails{
				SideCar: []types.SideCar{},
			},
			wantLen: 0,
		},
		{
			name: "sidecar without secrets returns empty volumes",
			chaosDetails: types.ChaosDetails{
				SideCar: []types.SideCar{
					{Name: "helper", Image: "busybox:latest"},
				},
			},
			wantLen: 0,
		},
		{
			name: "sidecar with one secret creates one volume",
			chaosDetails: types.ChaosDetails{
				SideCar: []types.SideCar{
					{
						Name: "helper",
						Secrets: []v1alpha1.Secret{
							{Name: "my-secret", MountPath: "/secrets"},
						},
					},
				},
			},
			wantLen:   1,
			wantNames: []string{"my-secret"},
		},
		{
			name: "duplicate secrets across sidecars are deduplicated",
			chaosDetails: types.ChaosDetails{
				SideCar: []types.SideCar{
					{
						Name: "sidecar-1",
						Secrets: []v1alpha1.Secret{
							{Name: "shared-secret", MountPath: "/secrets"},
						},
					},
					{
						Name: "sidecar-2",
						Secrets: []v1alpha1.Secret{
							{Name: "shared-secret", MountPath: "/secrets"},
						},
					},
				},
			},
			wantLen:   1,
			wantNames: []string{"shared-secret"},
		},
		{
			name: "unique secrets across sidecars all included",
			chaosDetails: types.ChaosDetails{
				SideCar: []types.SideCar{
					{
						Name: "sidecar-1",
						Secrets: []v1alpha1.Secret{
							{Name: "secret-a", MountPath: "/a"},
						},
					},
					{
						Name: "sidecar-2",
						Secrets: []v1alpha1.Secret{
							{Name: "secret-b", MountPath: "/b"},
						},
					},
				},
			},
			wantLen:   2,
			wantNames: []string{"secret-a", "secret-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSidecarVolumes(&tt.chaosDetails)
			if len(got) != tt.wantLen {
				t.Errorf("GetSidecarVolumes() len=%d, want %d", len(got), tt.wantLen)
				return
			}
			for i, v := range got {
				if i < len(tt.wantNames) && v.Name != tt.wantNames[i] {
					t.Errorf("GetSidecarVolumes()[%d].Name = %q, want %q", i, v.Name, tt.wantNames[i])
				}
				// Verify the volume has a Secret source with the correct name
				if v.VolumeSource.Secret == nil {
					t.Errorf("GetSidecarVolumes()[%d].VolumeSource.Secret is nil", i)
				} else if v.VolumeSource.Secret.SecretName != v.Name {
					t.Errorf("GetSidecarVolumes()[%d].VolumeSource.Secret.SecretName = %q, want %q",
						i, v.VolumeSource.Secret.SecretName, v.Name)
				}
			}
		})
	}
}

func TestHelperFailedError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		appLabel  string
		namespace string
		podLevel  bool
		wantCode  cerrors.ErrorType
	}{
		{
			name:      "nil error at pod level returns HelperPodFailed error",
			err:       nil,
			appLabel:  "app=nginx",
			namespace: "default",
			podLevel:  true,
			wantCode:  cerrors.ErrorTypeHelperPodFailed,
		},
		{
			name:      "nil error not at pod level returns generic error",
			err:       nil,
			appLabel:  "app=nginx",
			namespace: "default",
			podLevel:  false,
			wantCode:  cerrors.ErrorTypeGeneric,
		},
		{
			name:      "non-nil error gets propagated regardless of podLevel",
			err:       errors.New("original error"),
			appLabel:  "app=nginx",
			namespace: "default",
			podLevel:  true,
			wantCode:  cerrors.ErrorTypeNonUserFriendly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HelperFailedError(tt.err, tt.appLabel, tt.namespace, tt.podLevel)
			if result == nil {
				t.Error("HelperFailedError() should never return nil")
				return
			}
			gotCode := cerrors.GetErrorType(result)
			if gotCode != tt.wantCode {
				t.Errorf("HelperFailedError() error type = %v, want %v", gotCode, tt.wantCode)
			}
			// Verify target is included for cerrors.Error types
			if tt.err == nil {
				errMsg := result.Error()
				if !strings.Contains(errMsg, tt.appLabel) {
					t.Errorf("HelperFailedError() message %q does not contain appLabel %q", errMsg, tt.appLabel)
				}
				if !strings.Contains(errMsg, tt.namespace) {
					t.Errorf("HelperFailedError() message %q does not contain namespace %q", errMsg, tt.namespace)
				}
			}
		})
	}
}

func TestFilterBasedOnPercentage(t *testing.T) {
	tests := []struct {
		name       string
		percentage int
		list       []string
		wantLen    int
	}{
		{
			name:       "100 percent returns all items",
			percentage: 100,
			list:       []string{"a", "b", "c", "d"},
			wantLen:    4,
		},
		{
			name:       "50 percent of 4 items returns 2",
			percentage: 50,
			list:       []string{"a", "b", "c", "d"},
			wantLen:    2,
		},
		{
			name:       "25 percent of 4 items returns 1",
			percentage: 25,
			list:       []string{"a", "b", "c", "d"},
			wantLen:    1,
		},
		{
			name:       "0 percent returns at least 1 item (minimum)",
			percentage: 0,
			list:       []string{"a", "b", "c"},
			wantLen:    1,
		},
		{
			name:       "single item list returns that item",
			percentage: 50,
			list:       []string{"only-one"},
			wantLen:    1,
		},
		{
			name:       "100 percent of single item returns 1",
			percentage: 100,
			list:       []string{"only-one"},
			wantLen:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterBasedOnPercentage(tt.percentage, tt.list)
			if len(got) != tt.wantLen {
				t.Errorf("FilterBasedOnPercentage(%d, %v) len=%d, want %d; got %v",
					tt.percentage, tt.list, len(got), tt.wantLen, got)
			}
			// Verify all returned items are from the original list
			for _, item := range got {
				if !Contains(item, tt.list) {
					t.Errorf("FilterBasedOnPercentage() returned item %q not in original list %v", item, tt.list)
				}
			}
		})
	}
}

func TestValidateRange(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectExact string // if non-empty, expect this exact value
		expectRange [2]int // if set, expect result within [min, max]
		checkRange  bool
	}{
		{
			name:        "single value returns as-is",
			input:       "5",
			expectExact: "5",
		},
		{
			name:        "empty string returns as-is",
			input:       "",
			expectExact: "",
		},
		{
			name:        "non-numeric single value returns as-is",
			input:       "abc",
			expectExact: "abc",
		},
		{
			name:        "range returns value within bounds",
			input:       "1-10",
			checkRange:  true,
			expectRange: [2]int{1, 10},
		},
		{
			name:        "range with same bounds",
			input:       "5-5",
			checkRange:  true,
			expectRange: [2]int{5, 5},
		},
		{
			name:        "more than one dash returns 0",
			input:       "1-2-3",
			expectExact: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateRange(tt.input)
			if tt.expectExact != "" || (!tt.checkRange && tt.input == tt.expectExact) {
				if tt.expectExact != "" && got != tt.expectExact {
					t.Errorf("ValidateRange(%q) = %q, want %q", tt.input, got, tt.expectExact)
				}
			}
			if tt.checkRange {
				val := 0
				if _, err := parseInt(got, &val); err != nil {
					t.Errorf("ValidateRange(%q) = %q, expected numeric value in range [%d, %d]",
						tt.input, got, tt.expectRange[0], tt.expectRange[1])
					return
				}
				if val < tt.expectRange[0] || val > tt.expectRange[1] {
					t.Errorf("ValidateRange(%q) = %d, want value in [%d, %d]",
						tt.input, val, tt.expectRange[0], tt.expectRange[1])
				}
			}
		})
	}
}

func parseInt(s string, out *int) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0, err
	}
	*out = n
	return n, nil
}
