package statuscode

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var acceptedStatusCodes = []int{200, 201, 202, 204, 300, 301, 302, 304, 307, 400, 401, 403, 404, 500, 501, 502, 503, 504}

//PodHttpStatusCodeChaos contains the steps to prepare and inject http status code chaos
func PodHttpStatusCodeChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
		"Target Port":        experimentsDetails.TargetServicePort,
		"Listen Port":        experimentsDetails.ProxyPort,
		"Sequence":           experimentsDetails.Sequence,
		"PodsAffectedPerc":   experimentsDetails.PodsAffectedPerc,
		"Toxicity":           experimentsDetails.Toxicity,
		"StatusCode":         experimentsDetails.StatusCode,
		"ModifyResponseBody": experimentsDetails.ModifyResponseBody,
	})

	args := fmt.Sprintf("-t status_code -a status_code=%d -a modify_response_body=%d", experimentsDetails.StatusCode, stringBoolToInt(experimentsDetails.ModifyResponseBody))
	return http_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}

// GetStatusCode performs two functions:
// 1. It checks if the status code is provided or not. If it's not then it selects a random status code from supported list
// 2. It checks if the provided status code is valid or not.
func GetStatusCode(statusCode int) (int, error) {
	if statusCode == 0 {
		rand.Seed(time.Now().Unix())
		log.Info("[Info]: No status code provided. Selecting a status code randomly from supported status codes")
		return acceptedStatusCodes[1+rand.Intn(len(acceptedStatusCodes))], nil
	}
	if checkStatusCode(statusCode) {
		return statusCode, nil
	}
	return 0, errors.Errorf("status code %d is not supported. \nList of supported status code: %v", statusCode, acceptedStatusCodes)
}

// checkStatusCode checks if the provided status code is present in acceptedStatusCode list
func checkStatusCode(statusCode int) bool {
	for _, code := range acceptedStatusCodes {
		if code == statusCode {
			return true
		}
	}
	return false
}

// stringBoolToInt will convert boolean string to int
func stringBoolToInt(b string) int {
	parsedBool, err := strconv.ParseBool(b)
	if err != nil {
		return 0
	}
	if parsedBool {
		return 1
	}
	return 0
}
