package probe

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/log"
)

//Model contains operands and operator for the comparison operations
// a and b attribute belongs to operands and operator attribute belongs to operator
type Model struct {
	a        interface{}
	b        interface{}
	operator string
}

//FirstValue sets the first operands
func FirstValue(a interface{}) *Model {
	model := Model{}
	return model.FirstValue(a)
}

//FirstValue sets the first operands
func (model *Model) FirstValue(a interface{}) *Model {
	model.a = a
	return model
}

//SecondValue sets the second operand
func (model *Model) SecondValue(b interface{}) *Model {
	model.b = b
	return model
}

//Criteria sets the criteria/operator
func (model *Model) Criteria(criteria string) *Model {
	model.operator = criteria
	return model
}

// CompareInt compares integer numbers for specific operation
// it check for the >=, >, <=, <, ==, != operators
func (model Model) CompareInt() error {

	expectedOutput, err := strconv.Atoi(reflect.ValueOf(model.a).String())
	if err != nil {
		return err
	}
	actualOutput, err := strconv.Atoi(reflect.ValueOf(model.b).String())
	if err != nil {
		return err
	}

	switch model.operator {
	case ">=":
		if !(actualOutput >= expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "<=":
		if !(actualOutput <= expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case ">":
		if !(actualOutput > expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "<":
		if !(actualOutput < expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "==":
		if !(actualOutput == expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "!=":
		if !(actualOutput != expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	default:
		return fmt.Errorf("criteria '%s' not supported in the probe", model.operator)
	}
	return nil
}

// CompareFloat compares floating numbers for specific operation
// it check for the >=, >, <=, <, ==, != operators
func (model Model) CompareFloat() error {

	expectedOutput, err := strconv.ParseFloat(reflect.ValueOf(model.a).String(), 64)
	if err != nil {
		return err
	}
	actualOutput, err := strconv.ParseFloat(reflect.ValueOf(model.b).String(), 64)
	if err != nil {
		return err
	}
	switch model.operator {
	case ">=":
		if !(actualOutput >= expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "<=":
		if !(actualOutput <= expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case ">":
		if !(actualOutput > expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "<":
		if !(actualOutput < expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "==":
		if !(actualOutput == expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "!=":
		if !(actualOutput != expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	default:
		return fmt.Errorf("criteria '%s' not supported in the probe", model.operator)
	}
	return nil
}

// CompareString compares strings for specific operation
// it check for the equal, not equal and contains(sub-string) operations
func (model Model) CompareString() error {

	expectedOutput := reflect.ValueOf(model.a).String()
	actualOutput := reflect.ValueOf(model.b).String()

	log.Infof("actual: %v, expected: %v, operator: %v", actualOutput, expectedOutput, model.operator)

	switch model.operator {
	case "equal", "Equal":
		if !(actualOutput == expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "notEqual", "NotEqual":
		if !(actualOutput != expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "contains", "Contains":
		if !strings.Contains(actualOutput, expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "matches", "Matches":
		re, err := regexp.Compile(expectedOutput)
		if err != nil {
			return fmt.Errorf("The probe regex '%s' is not a valid expression", expectedOutput)
		}
		if !re.MatchString(actualOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "notMatches", "NotMatches":
		re, err := regexp.Compile(expectedOutput)
		if err != nil {
			return fmt.Errorf("The probe regex '%s' is not a valid expression", expectedOutput)
		}
		if re.MatchString(actualOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	default:
		return fmt.Errorf("criteria '%s' not supported in the probe", model.operator)
	}
	return nil
}
