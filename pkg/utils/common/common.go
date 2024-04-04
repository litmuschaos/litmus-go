package common

import (
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/machine/common/messages"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
)

// ENVDetails contains the ENV details
type ENVDetails struct {
	ENV []apiv1.EnvVar
}

//WaitForDuration waits for the given time duration (in seconds)
func WaitForDuration(duration int) {
	time.Sleep(time.Duration(duration) * time.Second)
}

// WaitForDurationAndCheckLiveness waits for a given chaos interval while validating a liveness criteria at certain intervals
func WaitForDurationAndCheckLiveness(connections []*websocket.Conn, agentEndpointList []string, chaosInterval int, abort chan os.Signal, chaosRevert *sync.WaitGroup) error {

	writeWait := time.Duration(10 * time.Second)

	ticker := time.NewTicker(5 * time.Second)

	chaosIntervalTimer := time.After(time.Duration(chaosInterval) * time.Second)

	for {
		select {

		case <-chaosIntervalTimer:
			return nil

		case <-abort:
			chaosRevert.Wait()

		case <-ticker.C:

			for i, conn := range connections {

				feedback, payload, err := messages.SendMessageToAgent(conn, "CHECK_LIVENESS", nil, &writeWait)
				if err != nil {
					return err
				}

				select {

				case <-abort:
					chaosRevert.Wait()

				default:
					if err := messages.ValidateAgentFeedback(feedback, payload); err != nil {
						return errors.Errorf("error during liveness check for %s agent endpoint, err: %v", agentEndpointList[i], err)
					}
				}
			}
		}
	}
}

// RandomInterval wait for the random interval lies between lower & upper bounds
func RandomInterval(interval string) error {
	intervals := strings.Split(interval, "-")
	var lowerBound, upperBound int
	switch len(intervals) {
	case 1:
		lowerBound = 0
		upperBound, _ = strconv.Atoi(intervals[0])
	case 2:
		lowerBound, _ = strconv.Atoi(intervals[0])
		upperBound, _ = strconv.Atoi(intervals[1])
	default:
		return errors.Errorf("unable to parse CHAOS_INTERVAL, provide in valid format")
	}
	rand.Seed(time.Now().UnixNano())
	waitTime := lowerBound + rand.Intn(upperBound-lowerBound)
	log.Infof("[Wait]: Wait for the random chaos interval %vs", waitTime)
	WaitForDuration(waitTime)
	return nil
}

// GetRunID generate a random string
func GetRunID() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")
	runID := make([]rune, 6)
	rand.Seed(time.Now().UnixNano())
	for i := range runID {
		runID[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(runID)
}

// AbortWatcher continuously watch for the abort signals
// it will update chaosresult w/ failed step and create an abort event, if it received abort signal during chaos
func AbortWatcher(expname string, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails) {
	AbortWatcherWithoutExit(expname, clients, resultDetails, chaosDetails, eventsDetails)
	os.Exit(1)
}

// AbortWatcherWithoutExit continuously watch for the abort signals
func AbortWatcherWithoutExit(expname string, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails) {

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM)

	// waiting until the abort signal received
	<-signChan

	log.Info("[Chaos]: Chaos Experiment Abortion started because of terminated signal received")
	// updating the chaosresult after stopped
	failStep := "Chaos injection stopped!"
	types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
	result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

	// generating summary event in chaosengine
	msg := expname + " experiment has been aborted"
	types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
	events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

	// generating summary event in chaosresult
	types.SetResultEventAttributes(eventsDetails, types.AbortVerdict, msg, "Warning", resultDetails)
	events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")
}

//GetIterations derive the iterations value from given parameters
func GetIterations(duration, interval int) int {
	var iterations int
	if interval != 0 {
		iterations = duration / interval
	}
	return math.Maximum(iterations, 1)
}

//FilterBasedOnPercentage return the slice of list based on the the provided percentage
func FilterBasedOnPercentage(percentage int, list []string) []string {

	var finalList []string
	newInstanceListLength := math.Maximum(1, math.Adjustment(percentage, len(list)))
	rand.Seed(time.Now().UnixNano())

	// it will generate the random instanceList
	// it starts from the random index and choose requirement no of volumeID next to that index in a circular way.
	index := rand.Intn(len(list))
	for i := 0; i < newInstanceListLength; i++ {
		finalList = append(finalList, list[index])
		index = (index + 1) % len(list)
	}
	return finalList
}

// SetEnv sets the env inside envDetails struct
func (envDetails *ENVDetails) SetEnv(key, value string) *ENVDetails {
	if value != "" {
		envDetails.ENV = append(envDetails.ENV, apiv1.EnvVar{
			Name:  key,
			Value: value,
		})
	}
	return envDetails
}

// SetEnvFromDownwardAPI sets the downapi env in envDetails struct
func (envDetails *ENVDetails) SetEnvFromDownwardAPI(apiVersion string, fieldPath string) *ENVDetails {
	if apiVersion != "" && fieldPath != "" {
		// Getting experiment pod name from downward API
		experimentPodName := getEnvSource(apiVersion, fieldPath)
		envDetails.ENV = append(envDetails.ENV, apiv1.EnvVar{
			Name:      "POD_NAME",
			ValueFrom: &experimentPodName,
		})
	}
	return envDetails
}

// getEnvSource return the env source for the given apiVersion & fieldPath
func getEnvSource(apiVersion string, fieldPath string) apiv1.EnvVarSource {
	downwardENV := apiv1.EnvVarSource{
		FieldRef: &apiv1.ObjectFieldSelector{
			APIVersion: apiVersion,
			FieldPath:  fieldPath,
		},
	}
	return downwardENV
}

// HelperFailedError return the helper pod error message
func HelperFailedError(err error) error {
	if err != nil {
		return errors.Errorf("helper pod failed, err: %v", err)
	}
	return errors.Errorf("helper pod failed")
}

// GetStatusMessage returns the event message
func GetStatusMessage(defaultCheck bool, defaultMsg, probeStatus string) string {
	if defaultCheck {
		if probeStatus == "" {
			return defaultMsg
		}
		return defaultMsg + ", Probes: " + probeStatus
	}
	if probeStatus == "" {
		return "Skipped the default checks"
	}
	return "Probes: " + probeStatus
}

//GetRandomSequence will gives a random value for sequence
func GetRandomSequence(sequence string) string {
	if strings.ToLower(sequence) == "random" {
		rand.Seed(time.Now().UnixNano())
		seq := []string{"serial", "parallel"}
		randomIndex := rand.Intn(len(seq))
		return seq[randomIndex]
	}
	return sequence
}

//ValidateRange validates the given range of numbers
func ValidateRange(a string) string {
	var lb, ub int
	intervals := strings.Split(a, "-")

	switch len(intervals) {
	case 1:
		return a
	case 2:
		lb, _ = strconv.Atoi(intervals[0])
		ub, _ = strconv.Atoi(intervals[1])
		return strconv.Itoa(getRandomValue(lb, ub))
	default:
		log.Errorf("unable to parse the value, please provide in valid format")
		return "0"
	}
}

//getRandomValue gives a random value between two integers
func getRandomValue(a, b int) int {
	rand.Seed(time.Now().Unix())
	return (a + rand.Intn(b-a+1))
}
