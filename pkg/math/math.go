package math

// Maximum calculates the maximum value among two integers
func Maximum(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

//Minimum calculates the minimum value among two integers
func Minimum(a int, b int) int {
	if a > b {
		return b
	}
	return a
}

//Rule of three for calculating an integer given another integer representing a percentage
func Adjustment(a int, b int) int {

	b = a * b / 100

	return b
}
