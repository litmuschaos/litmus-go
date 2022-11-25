package cerrors

import (
	"encoding/json"
	"github.com/palantir/stacktrace"
)

type ErrorType string

const (
	ErrorTypeNonUserFriendly   ErrorType = "NON_USER_FRIENDLY_ERROR"
	ErrorTypeGeneric           ErrorType = "GENERIC_ERROR"
	ErrorTypeChaosResultCRUD   ErrorType = "CHAOS_RESULT_CRUD_ERROR"
	ErrorTypePodStatusChecks   ErrorType = "POD_STATUS_CHECKS_ERROR"
	ErrorTypeTargetSelection   ErrorType = "TARGET_SELECTION_ERROR"
	ErrorTypeExperimentAborted ErrorType = "EXPERIMENT_ABORTED"
	ErrorTypeNodeStatusChecks  ErrorType = "NODE_STATUS_CHECKS_ERROR"
)

type userFriendly interface {
	UserFriendly() bool
	ErrorType() ErrorType
}

// IsUserFriendly returns true if err is marked as safe to present to failstep
func IsUserFriendly(err error) bool {
	ufe, ok := err.(userFriendly)
	return ok && ufe.UserFriendly()
}

// GetErrorType returns the type of error if the error is user-friendly
func GetErrorType(err error) ErrorType {
	if ufe, ok := err.(userFriendly); ok {
		return ufe.ErrorType()
	}
	return ErrorTypeNonUserFriendly
}

func GetRootCauseAndErrorCode(err error) (string, ErrorType) {
	rootCause := stacktrace.RootCause(err)
	errorType := GetErrorType(rootCause)
	if !IsUserFriendly(rootCause) {
		return err.Error(), errorType
	}
	return rootCause.Error(), errorType
}

type Error struct {
	ErrorCode ErrorType `json:"errorCode,omitempty"`
	Phase     string    `json:"phase,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	Target    string    `json:"target,omitempty"`
}

func (e Error) Error() string {
	return convertToJson(e)
}

func (e Error) UserFriendly() bool {
	return true
}

func (e Error) ErrorType() ErrorType {
	return e.ErrorCode
}

func convertToJson(v interface{}) string {
	vStr, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(vStr)
}
