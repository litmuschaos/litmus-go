package cerrors

import "github.com/palantir/stacktrace"

type ErrorType string

const (
	ErrorTypeNonUserFriendly         ErrorType = "NON_USER_FRIENDLY_ERROR"
	ErrorTypeGeneric                 ErrorType = "GENERIC_ERROR"
	ErrorTypeChaosResultCRUD         ErrorType = "CHAOS_RESULT_CRUD_ERROR"
	ErrorTypeApplicationStatusChecks ErrorType = "APPLICATION_STATUS_CHECKS_ERROR"
	ErrorTypeTargetSelection         ErrorType = "TARGET_SELECTION_ERROR"
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
