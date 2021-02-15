package probe

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"

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

// typeCastToInt type cast the interface argument to integer value
func typeCastToInt(a interface{}) (int, error) {
	return strconv.Atoi(reflect.ValueOf(a).String())
}

// typeCastToFloat type cast the interface argument to float64 value
func typeCastToFloat(a interface{}) (float64, error) {
	return strconv.ParseFloat(reflect.ValueOf(a).String(), 64)
}

// CompareInt compares integer numbers for specific operation
// it check for the >=, >, <=, <, ==, != operators
func (model Model) CompareInt() error {

	// typecasting actual value to the integer type
	actualOutput, err := typeCastToInt(model.b)
	if err != nil {
		return err
	}

	switch strings.ToLower(model.operator) {
	case "oneof":
		expectedValues := reflect.ValueOf(model.a).String()
		expectedValueList := strings.Split(strings.TrimSpace(expectedValues), ",")
		// matches that the actual value should be present inside the given expected list
		for i := range expectedValueList {
			value, _ := strconv.Atoi(expectedValueList[i])
			if value == actualOutput {
				return nil
			}
		}
		return errors.Errorf("Actual value: {%v} doesn't matched with any of the expected values: {%v}", actualOutput, expectedValueList)
	}

	// typecasting expected value to be integer type
	expectedOutput, err := typeCastToInt(model.a)
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

	// typecasting actual value to the floating type
	actualOutput, err := typeCastToFloat(model.b)
	if err != nil {
		return err
	}

	switch strings.ToLower(model.operator) {
	case "oneof":
		expectedValues := reflect.ValueOf(model.a).String()
		expectedValueList := strings.Split(strings.TrimSpace(expectedValues), ",")
		// matches that the actual value should be present inside the given expected list
		for i := range expectedValueList {
			value, _ := strconv.ParseFloat(expectedValueList[i], 64)
			if value == actualOutput {
				return nil
			}
		}
		return errors.Errorf("Actual value: {%v} doesn't matched with any of the expected values: {%v}", actualOutput, expectedValueList)
	}

	// typecasting expected value to the floating type
	expectedOutput, err := typeCastToFloat(model.a)
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

	switch strings.ToLower(model.operator) {
	case "equal":
		if !(actualOutput == expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "notequal":
		if !(actualOutput != expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "contains":
		if !strings.Contains(actualOutput, expectedOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "matches":
		re, err := regexp.Compile(expectedOutput)
		if err != nil {
			return fmt.Errorf("The probe regex '%s' is not a valid expression", expectedOutput)
		}
		if !re.MatchString(actualOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "notmatches":
		re, err := regexp.Compile(expectedOutput)
		if err != nil {
			return fmt.Errorf("The probe regex '%s' is not a valid expression", expectedOutput)
		}
		if re.MatchString(actualOutput) {
			return fmt.Errorf("The probe output didn't match with expected criteria")
		}
	case "oneof":
		expectedValueList := strings.Split(strings.TrimSpace(expectedOutput), ",")
		for i := range expectedValueList {
			if expectedValueList[i] == actualOutput {
				return nil
			}
		}
		return errors.Errorf("Actual value: {%v} doesn't matched with any of the expected values: {%v}", actualOutput, expectedValueList)
	default:
		return fmt.Errorf("criteria '%s' not supported in the probe", model.operator)
	}
	return nil
}
