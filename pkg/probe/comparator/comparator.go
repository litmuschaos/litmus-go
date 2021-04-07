package comparator

//Model contains operands and operator for the comparison operations
// a and b attribute belongs to operands and operator attribute belongs to operator
type Model struct {
	a        interface{}
	b        interface{}
	operator string
	rc       int
}

//RunCount sets the run counts
func RunCount(rc int) *Model {
	model := Model{}
	return model.RunCount(rc)
}

//RunCount sets the run counts
func (model *Model) RunCount(rc int) *Model {
	model.rc = rc
	return model
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
