package math

import (
	"testing"
)

func TestMaximum(t *testing.T) {
	tests := []struct {
		name     string
		a, b     int
		expected int
	}{
		{"First number is greater", 10, 5, 10},
		{"Second number is greater", 3, 8, 8},
		{"Numbers are equal", 5, 5, 5},
		{"Negative numbers", -5, -2, -2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Maximum(tt.a, tt.b); got != tt.expected {
				t.Errorf("Maximum() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMinimum(t *testing.T) {
	tests := []struct {
		name     string
		a, b     int
		expected int
	}{
		{"First number is smaller", 3, 10, 3},
		{"Second number is smaller", 8, 2, 2},
		{"Numbers are equal", 5, 5, 5},
		{"Negative numbers", -5, -10, -10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Minimum(tt.a, tt.b); got != tt.expected {
				t.Errorf("Minimum() = %v, want %v", got, tt.expected)
			}
		})
	}
}
func TestAdjustment(t *testing.T) {
	tests := []struct {
		name       string
		value      int
		percentage int
		expected   int
	}{
		{"50 percent of 10", 10, 50, 5},
		{"100 percent of 5", 5, 100, 5},
		{"0 percent of 10", 10, 0, 0},
		{"33 percent of 10 (rounds down)", 10, 33, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Adjustment(tt.value, tt.percentage); got != tt.expected {
				t.Errorf("Adjustment() = %v, want %v", got, tt.expected)
			}
		})
	}
}
