package common

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func FuzzRandomInterval(f *testing.F) {
	testCases := []struct {
		interval string
	}{
		{
			interval: "13",
		},
	}

	for _, tc := range testCases {
		f.Add(tc.interval)
	}

	f.Fuzz(func(t *testing.T, interval string) {
		re := regexp.MustCompile(`^\d+(-\d+)?$`)
		intervals := strings.Split(interval, "-")
		err := RandomInterval(interval)

		if re.MatchString(interval) == false {
			assert.Error(t, err, "{\"errorCode\":\"GENERIC_ERROR\",\"reason\":\"could not parse CHAOS_INTERVAL env, bad input\"}")
		}

		num, _ := strconv.Atoi(intervals[0])
		if num < 1 && err != nil {
			assert.Error(t, err, "{\"errorCode\":\"GENERIC_ERROR\",\"reason\":\"invalid CHAOS_INTERVAL env value, value below lower limit\"}")
		} else if num > 1 && err != nil {
			t.Errorf("Unexpected Error: %v", err)
		}
	})
}

func FuzzGetContainerNames(f *testing.F) {

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			chaosDetails types.ChaosDetails
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}
		names := GetContainerNames(&targetStruct.chaosDetails)
		require.Equal(t, len(names), len(targetStruct.chaosDetails.SideCar)+1)
	})
}

func FuzzGetSidecarVolumes(f *testing.F) {

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			chaosDetails types.ChaosDetails
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}
		volumes := GetSidecarVolumes(&targetStruct.chaosDetails)
		var volCounts = 0
		for _, s := range targetStruct.chaosDetails.SideCar {
			volCounts += len(s.Secrets)
		}
		require.Equal(t, len(volumes), len(volumes))
	})
}

func FuzzBuildSidecar(f *testing.F) {

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			chaosDetails types.ChaosDetails
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}
		containers := BuildSidecar(&targetStruct.chaosDetails)
		require.Equal(t, len(containers), len(targetStruct.chaosDetails.SideCar))
	})
}

func FuzzContains(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			val   string
			slice []string
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}
		contains := Contains(targetStruct.val, targetStruct.slice)
		for _, s := range targetStruct.slice {
			if s == targetStruct.val {
				require.True(t, contains)
				return
			}
		}
		require.False(t, contains)
	})
}

func FuzzSubStringExistsInSlice(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			val   string
			slice []string
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}
		contains := SubStringExistsInSlice(targetStruct.val, targetStruct.slice)
		for _, s := range targetStruct.slice {
			if strings.Contains(s, targetStruct.val) {
				require.True(t, contains)
				return
			}
		}
		require.False(t, contains)
	})
}

func FuzzGetRandomSequence(f *testing.F) {
	f.Add("random")

	f.Fuzz(func(t *testing.T, sequence string) {
		val := GetRandomSequence(sequence)
		if strings.ToLower(sequence) == "random" {
			require.Contains(t, []string{"serial", "parallel"}, val)
			return
		}
		require.Equal(t, sequence, val)
	})
}
