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
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
)

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

loop:
	for {
		select {
		case <-signChan:
			log.Info("[Chaos]: Chaos Experiment Abortion started because of terminated signal received")
			// updating the chaosresult after stopped
			failStep := "Chaos injection stopped!"
			types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
			if err := result.ChaosResult(chaosDetails, clients, resultDetails, "EOT"); err != nil {
				log.Errorf("unable to update the chaosresult, err: %v", err)
			}

			// generating summary event in chaosengine
			msg := expname + " experiment has been aborted"
			types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

			// generating summary event in chaosresult
			types.SetResultEventAttributes(eventsDetails, types.Summary, msg, "Warning", resultDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")
			break loop
		}
	}
}
