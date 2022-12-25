package comparator

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/log"
)

// CompareString compares strings for specific operation
// it check for the equal, not equal and contains(sub-string) operations
func (model Model) CompareString(errorCode cerrors.ErrorType) error {

	obj := String{}
	obj.setValues(reflect.ValueOf(model.a).String(), reflect.ValueOf(model.b).String())

	if model.rc == 1 {
		log.Infof("[Probe]: {Actual value: %v}, {Expected value: %v}, {Operator: %v}", obj.a, obj.b, model.operator)
	}

	switch model.operator {
	case "equal", "Equal":
		if !obj.isEqual() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v is not equal to the Expected value: %v", obj.a, obj.b)}
		}
	case "notEqual", "NotEqual":
		if !obj.isNotEqual() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v should not matched with the Expected value: %v", obj.a, obj.b)}
		}
	case "contains", "Contains":
		if !obj.isContains() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v doesn't contain Expected value: %v", obj.a, obj.b)}
		}
	case "matches", "Matches":
		re, err := regexp.Compile(obj.b)
		if err != nil {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("The probe regex '%s' is not a valid expression", obj.b)}
		}
		if !obj.isMatched(re) {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v is not matched with the Expected regex: %v", obj.a, obj.b)}
		}
	case "notMatches", "NotMatches":
		re, err := regexp.Compile(obj.b)
		if err != nil {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("The probe regex '%s' is not a valid expression", obj.b)}
		}
		if obj.isMatched(re) {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v should not matched with Expected regex: %v", obj.a, obj.b)}
		}
	case "oneOf", "OneOf":
		if !obj.isOneOf() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v doesn't matched any of the the Expected values: %v", obj.a, obj.c)}
		}
	default:
		return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("criteria '%s' not supported in the probe", model.operator)}
	}
	return nil
}

// String contains operands for String comparator check
type String struct {
	a string
	b string
	c []string
}

// SetValues sets the values inside String struct
func (s *String) setValues(a, b string) {

	s.a = a
	c := strings.Split(strings.TrimSpace(b), ",")
	if len(c) > 1 {
		s.c = c
		s.b = ""
	} else {
		s.b = b
	}
}

// isEqual check for the first string should be equals to second string
func (s *String) isEqual() bool {
	return s.a == s.b
}

// isNotEqual check for the first string should be not equals to second string
func (s *String) isNotEqual() bool {
	return s.a != s.b
}

// isContains check for the first string should be substring of second string
func (s *String) isContains() bool {
	return strings.Contains(s.a, s.b)
}

// isMatched check for the first value should follow the given regex
func (s *String) isMatched(re *regexp.Regexp) bool {
	return re.MatchString(s.a)
}

// isOneOf check for the string should be present inside given list
func (s *String) isOneOf() bool {
	for i := range s.c {
		if s.a == s.c[i] {
			return true
		}
	}
	return false
}
