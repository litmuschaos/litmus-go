package stringutils

import (
	"math/rand"
	"strings"
	"time"
	"strings"
	"fmt"
	"encoding/json"
)

const (
	shaLetters    = "0123456789abcdefghijklmnopqrstuvwxyz"
	letterIdxBits = 6                    // 5 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func SplitList(s string) []string {

	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(strings.TrimSpace(s), ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func GetRunID() string {
	return RandStringBytesMask(6, rand.NewSource(time.Now().UnixNano()))
}

func RandStringBytesMask(n int, src rand.Source) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(shaLetters) {
			b[i] = shaLetters[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

func ParseHostnames(input string) (string, error) {

	if(input == "") {
		return input, nil
	}	

	var parsed []string

	// Try JSON parse first
	if(strings.HasPrefix(input, "[") && strings.HasSuffix(input, "]")) {
		err := json.Unmarshal([]byte(input), &parsed)
		if err == nil {
			// Input was valid JSON list of strings
			return input, nil
		} else {
			return "", fmt.Errorf("failed to parse input as JSON: %v", err)
		}
	}


	// If not in JSON format, fallback to comma-separated logic
	parts := strings.Split(input, ",")
	for part := range parts {
            part = strings.TrimSpace(part)
            if part != "" {
                parsed = append(parsed, part)
            }
        }

	// Rebuild as JSON array of strings
	jsonBytes, err := json.Marshal(parsed)
	if err != nil {
		return "", fmt.Errorf("failed to format names: %v", err)
	}

	return string(jsonBytes), nil
}
