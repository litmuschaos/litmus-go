package stringutils

import (
	"math/rand"
	"time"
	"strings"
	"fmt"
)

const (
	shaLetters    = "0123456789abcdefghijklmnopqrstuvwxyz"
	letterIdxBits = 6                    // 5 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

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

func FormatHostnames(input string) string {
	// Split the input by comma and trim spaces
	parts := strings.Split(input, ",")
	for i, name := range parts {
		parts[i] = fmt.Sprintf("\"%s\"", strings.TrimSpace(name))
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ", "))
}
