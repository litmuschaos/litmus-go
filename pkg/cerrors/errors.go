package cerrors

import "fmt"

type Generic struct {
	Phase  string
	Reason string
}

func (e Generic) Error() string {
	if e.Phase == "" {
		return e.Reason
	}
	return fmt.Sprintf("[%s]: %s", e.Phase, e.Reason)
}

func (e Generic) UserFriendly() bool {
	return true
}

func (e Generic) ErrorType() ErrorType {
	return ErrorTypeGeneric
}

type ChaosResultCRUD struct {
	Phase     string
	Target    string
	Operation string
	Reason    string
}

func (e ChaosResultCRUD) Error() string {
	if e.Phase == "" {
		return fmt.Sprintf("failed to %s chaosresult: '%s', %s", e.Operation, e.Target, e.Reason)
	}
	return fmt.Sprintf("[%s]: failed to %s chaosresult: '%s', %s", e.Phase, e.Operation, e.Target, e.Reason)
}

func (e ChaosResultCRUD) UserFriendly() bool {
	return true
}

func (e ChaosResultCRUD) ErrorType() ErrorType {
	return ErrorTypeChaosResultCRUD
}

type ApplicationStatusChecks struct {
	Target string
	Reason string
}

func (e ApplicationStatusChecks) Error() string {
	return fmt.Sprintf("application '%s' status check failed, %s", e.Target, e.Reason)
}

func (e ApplicationStatusChecks) UserFriendly() bool {
	return true
}

func (e ApplicationStatusChecks) ErrorType() ErrorType {
	return ErrorTypeApplicationStatusChecks
}

type TargetPodSelection struct {
	Target string
	Reason string
}

func (e TargetPodSelection) Error() string {
	if e.Target == "" {
		return fmt.Sprintf("target selection failed, %s", e.Reason)
	}
	return fmt.Sprintf("target '%s' selection failed, %s", e.Target, e.Reason)
}

func (e TargetPodSelection) UserFriendly() bool {
	return true
}

func (e TargetPodSelection) ErrorType() ErrorType {
	return ErrorTypeTargetSelection
}

type TargetDiskSelection struct {
	Target string
	Reason string
}

func (e TargetDiskSelection) Error() string {
	if e.Target == "" {
		return fmt.Sprintf("target selection failed, %s", e.Reason)
	}
	return fmt.Sprintf("target '%s' selection failed, %s", e.Target, e.Reason)
}

func (e TargetDiskSelection) UserFriendly() bool {
	return true
}

func (e TargetDiskSelection) ErrorType() ErrorType {
	return ErrorTypeTargetSelection
}
