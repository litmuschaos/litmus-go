package common

import (
	"github.com/stretchr/testify/assert"
	"regexp"
	"strconv"
	"strings"
	"testing"
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
