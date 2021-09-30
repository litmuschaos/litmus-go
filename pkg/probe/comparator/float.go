package comparator

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

// CompareFloat compares floating numbers for specific operation
// it check for the >=, >, <=, <, ==, != operators
func (model Model) CompareFloat() error {

	obj := Float{}
	obj.setValues(reflect.ValueOf(model.a).String(), reflect.ValueOf(model.b).String())

	if model.rc == 1 {
		log.Infof("[Probe]: {Actual value: %v}, {Expected value: %v}, {Operator: %v}", obj.a, obj.b, model.operator)
	}

	switch model.operator {
	case ">=":
		if !obj.isGreaterorEqual() {
			return errors.Errorf("{actual value: %v} is not greater than or equal to {expected value: %v}", obj.a, obj.b)
		}
	case "<=":
		if !obj.isLesserorEqual() {
			return errors.Errorf("{actual value: %v} is not lesser than or equal to {expected value: %v}", obj.a, obj.b)
		}
	case ">":
		if !obj.isGreater() {
			return errors.Errorf("{actual value: %v} is not greater than {expected value: %v}", obj.a, obj.b)
		}
	case "<":
		if !obj.isLesser() {
			return errors.Errorf("{actual value: %v} is not lesser than {expected value: %v}", obj.a, obj.b)
		}
	case "==":
		if !obj.isEqual() {
			return errors.Errorf("{actual value: %v} is not equal to {expected value: %v}", obj.a, obj.b)
		}
	case "!=":
		if !obj.isNotEqual() {
			return errors.Errorf("{actual value: %v} is not Notequal to {expected value: %v}", obj.a, obj.b)
		}
	case "OneOf", "oneOf":
		if !obj.isOneOf() {
			return errors.Errorf("Actual value: {%v} doesn't matched with any of the expected values: {%v}", obj.a, obj.c)
		}
	case "between", "Between":
		if len(obj.c) < 2 {
			return errors.Errorf("{expected value: %v} should contains both lower and upper limits", obj.c)
		}
		if !obj.isBetween() {
			return errors.Errorf("Actual value: {%v} doesn't lie in between expected range: {%v}", obj.a, obj.c)
		}
	default:
		return errors.Errorf("criteria '%s' not supported in the probe", model.operator)
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
