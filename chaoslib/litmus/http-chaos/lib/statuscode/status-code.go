package statuscode

import (
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	body "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib/modify-body"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
)

var acceptedStatusCodes = []string{
	"200", "201", "202", "204",
	"300", "301", "302", "304", "307",
	"400", "401", "403", "404",
	"500", "501", "502", "503", "504",
}

// PodHttpStatusCodeChaos contains the steps to prepare and inject http status code chaos
func PodHttpStatusCodeChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// responseBodyMaxLength defines the max length of response body string to be printed. It is taken as
	// the min of length of body and 120 characters to avoid printing large response body.
	responseBodyMaxLength := int(math.Min(float64(len(experimentsDetails.ResponseBody)), 120))

	log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
		"Target Port":        experimentsDetails.TargetServicePort,
		"Listen Port":        experimentsDetails.ProxyPort,
		"Sequence":           experimentsDetails.Sequence,
		"PodsAffectedPerc":   experimentsDetails.PodsAffectedPerc,
		"Toxicity":           experimentsDetails.Toxicity,
		"StatusCode":         experimentsDetails.StatusCode,
		"ModifyResponseBody": experimentsDetails.ModifyResponseBody,
		"ResponseBody":       experimentsDetails.ResponseBody[0:responseBodyMaxLength],
		"Content Type":       experimentsDetails.ContentType,
		"Content Encoding":   experimentsDetails.ContentEncoding,
	})

	args := fmt.Sprintf(
		`-t status_code -a status_code=%s -a modify_response_body=%d -a response_body="%v" -a content_type=%s -a content_encoding=%s`,
		experimentsDetails.StatusCode, stringBoolToInt(experimentsDetails.ModifyResponseBody), body.EscapeQuotes(experimentsDetails.ResponseBody),
		experimentsDetails.ContentType, experimentsDetails.ContentEncoding)
	return http_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}

// GetStatusCode performs two functions:
// 1. It checks if the status code is provided or not. If it's not then it selects a random status code from supported list
// 2. It checks if the provided status code is valid or not.
func GetStatusCode(statusCode string) (string, error) {

	if statusCode == "" {
		log.Info("[Info]: No status code provided. Selecting a status code randomly from supported status codes")
		return acceptedStatusCodes[rand.Intn(len(acceptedStatusCodes))], nil
	}

	statusCodeList := strings.Split(statusCode, ",")
	rand.Seed(time.Now().Unix())
	if len(statusCodeList) == 1 {
		if checkStatusCode(statusCodeList[0], acceptedStatusCodes) {
			return statusCodeList[0], nil
		}
	} else {
		acceptedCodes := getAcceptedCodesInList(statusCodeList, acceptedStatusCodes)
		if len(acceptedCodes) == 0 {
			return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("invalid status code: %s", statusCode)}
		}
		return acceptedCodes[rand.Intn(len(acceptedCodes))], nil
	}
	return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("status code '%s' is not supported. Supported status codes are: %v", statusCode, acceptedStatusCodes)}
}

// getAcceptedCodesInList returns the list of accepted status codes from a list of status codes
func getAcceptedCodesInList(statusCodeList []string, acceptedStatusCodes []string) []string {
	var acceptedCodes []string
	for _, statusCode := range statusCodeList {
		if checkStatusCode(statusCode, acceptedStatusCodes) {
			acceptedCodes = append(acceptedCodes, statusCode)
		}
	}
	return acceptedCodes
}

// checkStatusCode checks if the provided status code is present in acceptedStatusCode list
func checkStatusCode(statusCode string, acceptedStatusCodes []string) bool {
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
