package stringutils

import (
	"math/rand"
	"testing"
	"strings"
)

func TestRandStringBytesMask(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		seed     int64
		expected string
	}{
		{
			name:     "6-char string from seed 1234",
			n:        6,
			seed:     1234,
			expected: "ts9ng0",
		},
		{
			name:     "6-char string from seed 42",
			n:        6,
			seed:     42,
			expected: "pb6mvj",
		},
		{
			name:     "8-char string from seed 1234",
			n:        8,
			seed:     1234,
			expected: "9jts9ng0",
		},
		{
			name:     "empty string with n = 0",
			n:        0,
			seed:     999,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := rand.NewSource(tt.seed)
			got := RandStringBytesMask(tt.n, src)
			if got != tt.expected {
				t.Errorf("RandStringBytesMask(%d, %d) = %q; want %q", tt.n, tt.seed, got, tt.expected)
			}
		})
	}
}

func TestGetRunID(t *testing.T) {
	const shaLetters = "0123456789abcdefghijklmnopqrstuvwxyz"
	id := GetRunID()

	if len(id) != 6 {
		t.Errorf("expected length 6, got %d", len(id))
	}

	for _, ch := range id {
		if !strings.ContainsRune(shaLetters, ch) {
			t.Errorf("invalid character %q in run ID", ch)
		}
	}
}
