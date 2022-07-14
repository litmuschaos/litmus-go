package utils

import (
	"math/rand"
	"strconv"

	"github.com/pkg/errors"
)

var acceptedStatusCodes = []int{200, 201, 202, 204, 300, 301, 302, 304, 307, 400, 401, 403, 404, 500, 501, 502, 503, 504}

func GetStatusCode(statusCode string) int {
	if statusCode == "" {
		return acceptedStatusCodes[rand.Intn(len(acceptedStatusCodes)-1)+1]
	}

	convertedStatusCode, _ := strconv.Atoi(statusCode)
	if exist, _ := CheckStatusCode(convertedStatusCode); exist {
		return convertedStatusCode
	}
	return 0
}

func CheckStatusCode(statusCode int) (bool, error) {
	for _, code := range acceptedStatusCodes {
		if code == statusCode {
			return true, nil
		}
	}
	return false, errors.Errorf("status code %s is not supported. \nList of supported status code: %v", statusCode, acceptedStatusCodes)
}
