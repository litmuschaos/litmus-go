package stringutils

import (
	"math/rand"
	"strings"
	"testing"
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

func TestSplitList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple comma seperation",
			input:    "pod1,pod2,pod3",
			expected: []string{"pod1", "pod2", "pod3"},
		},
		{
			name:     "seperation with spaces",
			input:    " pod1 , pod2 , pod3 ",
			expected: []string{"pod1", "pod2", "pod3"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "string with empty elements",
			input:    "pod1,,pod2, ,",
			expected: []string{"pod1", "pod2"},
		},
		{
			name:	  "only commas input",
			input:    ",,,",
			expected: []string{},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitList(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("SplitList(%q) length =%d; want %d", tt.input, len(got), len(tt.expected))
				return
			}
			
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("SplitList(%q)[%d] = %q; want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}	
}