package comparator

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

// CompareInt compares integer numbers for specific operation
// it check for the >=, >, <=, <, ==, != operators
func (model Model) CompareInt() error {

	obj := Integer{}
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
			return errors.Errorf("{actual value: %v} is not NotEqual to {expected value: %v}", obj.a, obj.b)
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

// Integer contains operands for integer comparator check
type Integer struct {
	a int
	b int
	c []int
}

// SetValues sets the value inside Integer struct
func (i *Integer) setValues(a, b string) {

	i.a, _ = strconv.Atoi(a)
	c := strings.Split(strings.TrimSpace(b), ",")
	if len(c) > 1 {
		list := []int{}
		for j := range c {
			x, _ := strconv.Atoi(c[j])
			list = append(list, x)
		}
		i.c = list
		i.b = 0
	} else {
		i.b, _ = strconv.Atoi(b)
	}
}

// isGreater check for the first number should be greater than second number
func (i *Integer) isGreater() bool {
	return i.a > i.b
}

// isGreaterorEqual check for the first number should be greater than or equals to the second number
func (i *Integer) isGreaterorEqual() bool {
	return i.isGreater() || i.isEqual()
}

// isLesser check for the first number should be lesser than second number
func (i *Integer) isLesser() bool {
	return i.a < i.b
}

// isLesserorEqual check for the first number should be less than or equals to the second number
func (i *Integer) isLesserorEqual() bool {
	return i.isLesser() || i.isEqual()
}

// isEqual check for the first number should be equals to the second number
func (i *Integer) isEqual() bool {
	return i.a == i.b
}

// isNotEqual check for the first number should be not equals to the second number
func (i *Integer) isNotEqual() bool {
	return i.a != i.b
}

// isOneOf check for the number should be present inside given list
func (i *Integer) isOneOf() bool {
	for j := range i.c {
		if i.a == i.c[j] {
			return true
		}
	}
	return false
}

// isBetween check for the number should be lie in the given range
func (i *Integer) isBetween() bool {
	if i.a >= i.c[0] && i.a <= i.c[1] {
		return true
	}
	return false
}
