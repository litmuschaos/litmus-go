package common

import (
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
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

// AbortWatcher continuosly watch for the abort signals
// it will update chaosresult w/ failed step and create an abort event, if it recieved abort signal during chaos
func AbortWatcher(expname string, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails) {
	AbortWatcherWithoutExit(expname, clients, resultDetails, chaosDetails, eventsDetails)
	os.Exit(1)
}

// AbortWatcherWithoutExit continuosly watch for the abort signals
func AbortWatcherWithoutExit(expname string, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails) {

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM)

	// waiting until the abort signal recieved
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

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
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
