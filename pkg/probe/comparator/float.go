package comparator

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/log"
)

// CompareFloat compares floating numbers for specific operation
// it check for the >=, >, <=, <, ==, != operators
func (model Model) CompareFloat(errorCode cerrors.ErrorType) error {

	obj := Float{}
	obj.setValues(reflect.ValueOf(model.a).String(), reflect.ValueOf(model.b).String())

	if model.rc == 1 {
		log.Infof("[Probe]: {Actual value: %v}, {Expected value: %v}, {Operator: %v}", obj.a, obj.b, model.operator)
	}

	switch model.operator {
	case ">=":
		if !obj.isGreaterorEqual() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v is not greater than or equal to the Expected value: %v", obj.a, obj.b)}
		}
	case "<=":
		if !obj.isLesserorEqual() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v is not lesser than or equal to the Expected value: %v", obj.a, obj.b)}
		}
	case ">":
		if !obj.isGreater() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v is not greater than the Expected value: %v", obj.a, obj.b)}
		}
	case "<":
		if !obj.isLesser() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v is not lesser than the Expected value: %v", obj.a, obj.b)}
		}
	case "==":
		if !obj.isEqual() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v is not equal to the Expected value: %v", obj.a, obj.b)}
		}
	case "!=":
		if !obj.isNotEqual() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v should not matched with the Expected value: %v", obj.a, obj.b)}
		}
	case "OneOf", "oneOf":
		if !obj.isOneOf() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v doesn't matched any of the Expected values: %v", obj.a, obj.c)}
		}
	case "between", "Between":
		if len(obj.c) < 2 {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Expected value: %v should contains both the lower and upper limits", obj.c)}
		}
		if !obj.isBetween() {
			return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("Probe responded with an invalid output. Actual value: %v doesn't lie in between the Expected range: %v", obj.a, obj.c)}
		}
	default:
		return cerrors.Error{ErrorCode: errorCode, Target: model.probeName, Reason: fmt.Sprintf("criteria '%s' not supported in the probe", model.operator)}
	}
	return nil
}

// Float contains operands for float comparator check
type Float struct {
	a float64
	b float64
	c []float64
}

// SetValues set the values inside Float struct
func (f *Float) setValues(a, b string) {

	f.a, _ = strconv.ParseFloat(a, 64)
	c := strings.Split(strings.TrimSpace(b), ",")
	if len(c) > 1 {
		list := []float64{}
		for j := range c {
			x, _ := strconv.ParseFloat(c[j], 64)
			list = append(list, x)
		}
		f.c = list
		f.b = float64(0)
	} else {
		f.b, _ = strconv.ParseFloat(b, 64)
	}
}

// isGreater check for the first number should be greater than second number
func (f *Float) isGreater() bool {
	return f.a > f.b
}

// isGreaterorEqual check for the first number should be greater than or equals to the second number
func (f *Float) isGreaterorEqual() bool {
	return f.isGreater() || f.isEqual()
}

// isLesser check for the first number should be lesser than second number
func (f *Float) isLesser() bool {
	return f.a < f.b
}

// isLesserorEqual check for the first number should be less than or equals to the second number
func (f *Float) isLesserorEqual() bool {
	return f.isLesser() || f.isEqual()
}

// isEqual check for the first number should be equals to the second number
func (f *Float) isEqual() bool {
	return f.a == f.b
}

// isNotEqual check for the first number should be not equals to the second number
func (f *Float) isNotEqual() bool {
	return f.a != f.b
}

// isOneOf check for the number should be present inside given list
func (f *Float) isOneOf() bool {
	for i := range f.c {
		if f.a == f.c[i] {
			return true
		}
	}
	return false
}

// isBetween check for the number should be lie in the given range
func (f *Float) isBetween() bool {
	if f.a >= f.c[0] && f.a <= f.c[1] {
		return true
	}
	return false
}
