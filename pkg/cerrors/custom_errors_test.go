package cerrors

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/palantir/stacktrace"
)

func TestErrorTypeConstants(t *testing.T) {
	tests := []struct {
		name      string
		errorType ErrorType
		expected  string
	}{
		{"NonUserFriendly", ErrorTypeNonUserFriendly, "NON_USER_FRIENDLY_ERROR"},
		{"Generic", ErrorTypeGeneric, "GENERIC_ERROR"},
		{"ChaosResultCRUD", ErrorTypeChaosResultCRUD, "CHAOS_RESULT_CRUD_ERROR"},
		{"StatusChecks", ErrorTypeStatusChecks, "STATUS_CHECKS_ERROR"},
		{"TargetSelection", ErrorTypeTargetSelection, "TARGET_SELECTION_ERROR"},
		{"ExperimentAborted", ErrorTypeExperimentAborted, "EXPERIMENT_ABORTED"},
		{"Helper", ErrorTypeHelper, "HELPER_ERROR"},
		{"HelperPodFailed", ErrorTypeHelperPodFailed, "HELPER_POD_FAILED_ERROR"},
		{"ContainerRuntime", ErrorTypeContainerRuntime, "CONTAINER_RUNTIME_ERROR"},
		{"ChaosInject", ErrorTypeChaosInject, "CHAOS_INJECT_ERROR"},
		{"ChaosRevert", ErrorTypeChaosRevert, "CHAOS_REVERT_ERROR"},
		{"K8sProbe", ErrorTypeK8sProbe, "K8S_PROBE_ERROR"},
		{"FailureK8sProbe", FailureTypeK8sProbe, "K8S_PROBE_FAILURE"},
		{"CmdProbe", ErrorTypeCmdProbe, "CMD_PROBE_ERROR"},
		{"FailureCmdProbe", FailureTypeCmdProbe, "CMD_PROBE_FAILURE"},
		{"HttpProbe", ErrorTypeHttpProbe, "HTTP_PROBE_ERROR"},
		{"FailureHttpProbe", FailureTypeHttpProbe, "HTTP_PROBE_FAILURE"},
		{"PromProbe", ErrorTypePromProbe, "PROM_PROBE_ERROR"},
		{"FailurePromProbe", FailureTypePromProbe, "PROM_PROBE_FAILURE"},
		{"Timeout", ErrorTypeTimeout, "TIMEOUT"},
		{"ProbeTimeout", FailureTypeProbeTimeout, "PROBE_TIMEOUT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.errorType) != tt.expected {
				t.Errorf("ErrorType %s = %q; want %q", tt.name, string(tt.errorType), tt.expected)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  Error
	}{
		{
			name: "full error with all fields",
			err: Error{
				Source:    "test-source",
				ErrorCode: ErrorTypeGeneric,
				Phase:     "PreChaos",
				Reason:    "test reason",
				Target:    "test-pod",
			},
		},
		{
			name: "error with only error code and reason",
			err: Error{
				ErrorCode: ErrorTypeTimeout,
				Reason:    "operation timed out",
			},
		},
		{
			name: "empty error",
			err:  Error{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			// Verify it is valid JSON
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(result), &m); err != nil {
				t.Errorf("Error.Error() returned invalid JSON: %v, got: %s", err, result)
			}
			// Verify fields are present if non-empty
			if tt.err.ErrorCode != "" {
				if m["errorCode"] != string(tt.err.ErrorCode) {
					t.Errorf("expected errorCode=%q, got %v", tt.err.ErrorCode, m["errorCode"])
				}
			}
			if tt.err.Reason != "" {
				if m["reason"] != tt.err.Reason {
					t.Errorf("expected reason=%q, got %v", tt.err.Reason, m["reason"])
				}
			}
		})
	}
}

func TestError_UserFriendly(t *testing.T) {
	e := Error{ErrorCode: ErrorTypeGeneric, Reason: "test"}
	if !e.UserFriendly() {
		t.Error("Error.UserFriendly() should always return true")
	}
}

func TestError_ErrorType(t *testing.T) {
	tests := []struct {
		name      string
		err       Error
		wantType  ErrorType
	}{
		{"generic type", Error{ErrorCode: ErrorTypeGeneric}, ErrorTypeGeneric},
		{"timeout type", Error{ErrorCode: ErrorTypeTimeout}, ErrorTypeTimeout},
		{"status checks type", Error{ErrorCode: ErrorTypeStatusChecks}, ErrorTypeStatusChecks},
		{"chaos inject type", Error{ErrorCode: ErrorTypeChaosInject}, ErrorTypeChaosInject},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.ErrorType(); got != tt.wantType {
				t.Errorf("Error.ErrorType() = %v, want %v", got, tt.wantType)
			}
		})
	}
}

func TestIsUserFriendly(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "cerrors.Error is user-friendly",
			err:      Error{ErrorCode: ErrorTypeGeneric, Reason: "test"},
			expected: true,
		},
		{
			name:     "PreserveError is user-friendly",
			err:      PreserveError{ErrString: "preserved error"},
			expected: true,
		},
		{
			name:     "standard errors.New is not user-friendly",
			err:      errors.New("standard error"),
			expected: false,
		},
		{
			name:     "fmt.Errorf is not user-friendly",
			err:      errors.New("fmt error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUserFriendly(tt.err); got != tt.expected {
				t.Errorf("IsUserFriendly() = %v, want %v for error: %v", got, tt.expected, tt.err)
			}
		})
	}
}

func TestGetErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorType
	}{
		{
			name:     "cerrors.Error returns its ErrorCode",
			err:      Error{ErrorCode: ErrorTypeGeneric},
			expected: ErrorTypeGeneric,
		},
		{
			name:     "cerrors.Error with timeout returns timeout type",
			err:      Error{ErrorCode: ErrorTypeTimeout},
			expected: ErrorTypeTimeout,
		},
		{
			name:     "PreserveError returns ErrorTypeGeneric",
			err:      PreserveError{ErrString: "preserved"},
			expected: ErrorTypeGeneric,
		},
		{
			name:     "standard error returns NonUserFriendly",
			err:      errors.New("standard error"),
			expected: ErrorTypeNonUserFriendly,
		},
		{
			name:     "wrapped cerrors.Error via stacktrace returns its ErrorCode",
			err:      stacktrace.Propagate(Error{ErrorCode: ErrorTypeChaosInject, Reason: "inject failed"}, "wrapped"),
			expected: ErrorTypeChaosInject,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetErrorType(tt.err); got != tt.expected {
				t.Errorf("GetErrorType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetRootCauseAndErrorCode(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		phase         string
		wantType      ErrorType
		wantMsgContain string
	}{
		{
			name:           "cerrors.Error with phase already set",
			err:            Error{ErrorCode: ErrorTypeGeneric, Reason: "test reason", Phase: "PreChaos"},
			phase:          "PostChaos",
			wantType:       ErrorTypeGeneric,
			wantMsgContain: "test reason",
		},
		{
			name:           "cerrors.Error with empty phase gets phase set",
			err:            Error{ErrorCode: ErrorTypeStatusChecks, Reason: "status failed", Phase: ""},
			phase:          "PreChaosCheck",
			wantType:       ErrorTypeStatusChecks,
			wantMsgContain: "status failed",
		},
		{
			name:           "standard error returns non-user-friendly",
			err:            errors.New("raw error message"),
			phase:          "ChaosInject",
			wantType:       ErrorTypeNonUserFriendly,
			wantMsgContain: "raw error message",
		},
		{
			name:           "PreserveError returns generic type",
			err:            PreserveError{ErrString: "preserved error string"},
			phase:          "PostChaos",
			wantType:       ErrorTypeGeneric,
			wantMsgContain: "preserved error string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, errType := GetRootCauseAndErrorCode(tt.err, tt.phase)
			if errType != tt.wantType {
				t.Errorf("GetRootCauseAndErrorCode() ErrorType = %v, want %v", errType, tt.wantType)
			}
			if tt.wantMsgContain != "" && !strings.Contains(msg, tt.wantMsgContain) {
				t.Errorf("GetRootCauseAndErrorCode() msg = %q, want it to contain %q", msg, tt.wantMsgContain)
			}
		})
	}
}

func TestPreserveError_Error(t *testing.T) {
	tests := []struct {
		name     string
		pe       PreserveError
		expected string
	}{
		{"simple message", PreserveError{ErrString: "something went wrong"}, "something went wrong"},
		{"empty message", PreserveError{ErrString: ""}, ""},
		{"JSON-like message", PreserveError{ErrString: `{"key":"value"}`}, `{"key":"value"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pe.Error(); got != tt.expected {
				t.Errorf("PreserveError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestPreserveError_UserFriendly(t *testing.T) {
	pe := PreserveError{ErrString: "test"}
	if !pe.UserFriendly() {
		t.Error("PreserveError.UserFriendly() should always return true")
	}
}

func TestPreserveError_ErrorType(t *testing.T) {
	pe := PreserveError{ErrString: "test"}
	if got := pe.ErrorType(); got != ErrorTypeGeneric {
		t.Errorf("PreserveError.ErrorType() = %v, want %v", got, ErrorTypeGeneric)
	}
}

func TestError_ImplementsErrorInterface(t *testing.T) {
	var err error = Error{ErrorCode: ErrorTypeGeneric, Reason: "test"}
	if err == nil {
		t.Error("Error should implement error interface")
	}
}

func TestPreserveError_ImplementsErrorInterface(t *testing.T) {
	var err error = PreserveError{ErrString: "test"}
	if err == nil {
		t.Error("PreserveError should implement error interface")
	}
}
