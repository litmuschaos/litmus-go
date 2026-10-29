package lib

import (
	"testing"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-memory-hog/types"
)

func TestCalculateMemoryConsumption(t *testing.T) {
	const (
		gib               = 1024 * 1024 * 1024
		memoryCapacity    = 8 * gib
		memoryAllocatable = 8 * gib
	)

	tests := []struct {
		name       string
		percentage string
		mebibytes  string
		want       string
		wantErr    bool
	}{
		{name: "both unset with empty values uses default", percentage: "", mebibytes: "", want: "30%"},
		{name: "both unset with zero values uses default", percentage: "0", mebibytes: "0", want: "30%"},
		{name: "mebibytes with empty percentage", percentage: "", mebibytes: "2000", want: "2000m"},
		{name: "mebibytes with zero percentage", percentage: "0", mebibytes: "2000", want: "2000m"},
		{name: "mebibytes with blank percentage", percentage: " ", mebibytes: "2000", want: "2000m"},
		{name: "percentage with empty mebibytes", percentage: "50", mebibytes: "", want: "50%"},
		{name: "percentage with zero mebibytes", percentage: "50", mebibytes: "0", want: "50%"},
		{name: "percentage takes precedence when both set", percentage: "50", mebibytes: "2000", want: "50%"},
		{name: "mebibytes capped to allocatable memory", percentage: "", mebibytes: "100000", want: "8388608k"},
		{name: "invalid percentage returns error", percentage: "abc", mebibytes: "", wantErr: true},
		{name: "invalid mebibytes returns error", percentage: "", mebibytes: "2Gi", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := &experimentTypes.ExperimentDetails{
				MemoryConsumptionPercentage: tt.percentage,
				MemoryConsumptionMebibytes:  tt.mebibytes,
			}
			got, err := calculateMemoryConsumption(details, memoryCapacity, memoryAllocatable)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("calculateMemoryConsumption() = %q, want %q", got, tt.want)
			}
		})
	}
}
