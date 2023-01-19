package common

import (
	"bytes"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
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
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: "could not parse CHAOS_INTERVAL env, invalid format"}
	}
	rand.Seed(time.Now().UnixNano())
	waitTime := lowerBound + rand.Intn(upperBound-lowerBound)
	log.Infof("[Wait]: Wait for the random chaos interval %vs", waitTime)
	WaitForDuration(waitTime)
	return nil
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
	types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep, cerrors.ErrorTypeExperimentAborted)
	if err := result.ChaosResult(chaosDetails, clients, resultDetails, "EOT"); err != nil {
		log.Errorf("[ABORT]: Failed to update result, err: %v", err)
	}
	log.Info("[ABORT]: Updated chaosresult post stop")

	// generating summary event in chaosengine
	msg := expname + " experiment has been aborted"
	types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
	err := events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	if err != nil {
		log.Errorf("[ABORT]: Failed to create chaosengine summary event, err: %v", err)
	}

	// generating summary event in chaosresult
	types.SetResultEventAttributes(eventsDetails, types.AbortVerdict, msg, "Warning", resultDetails)
	err = events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")
	if err != nil {
		log.Errorf("[ABORT]: Failed to create chaosresult abort event, err: %v", err)
	}
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
func HelperFailedError(err error, appLabel, namespace string, podLevel bool) error {
	if err != nil {
		return stacktrace.Propagate(err, "helper pod failed")
	}
	if podLevel {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeHelperPodFailed, Target: fmt.Sprintf("{podLabel: %s, namespace: %s}", appLabel, namespace), Reason: "helper pod failed"}
	}
	return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{podLabel: %s, namespace: %s}", appLabel, namespace), Reason: "helper pod failed"}
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

// SubStringExistsInSlice checks the existence of sub string in slice
func SubStringExistsInSlice(val string, slice []string) bool {
	for _, v := range slice {
		if strings.Contains(val, v) {
			return true
		}
	}
	return false
}

func Contains(val interface{}, slice interface{}) bool {
	if slice == nil {
		return false
	}
	for i := 0; i < reflect.ValueOf(slice).Len(); i++ {
		if fmt.Sprintf("%v", reflect.ValueOf(val).Interface()) == fmt.Sprintf("%v", reflect.ValueOf(slice).Index(i).Interface()) {
			return true
		}
	}
	return false
}

func RunBashCommand(command string, failMsg string, source string) error {
	cmd := exec.Command("/bin/bash", "-c", command)
	return RunCLICommands(cmd, source, "", failMsg, cerrors.ErrorTypeHelper)
}

func RunCLICommands(cmd *exec.Cmd, source, target, failMsg string, errorCode cerrors.ErrorType) error {
	var out, stdErr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stdErr
	if err = cmd.Run(); err != nil {
		return cerrors.Error{ErrorCode: errorCode, Target: target, Source: source, Reason: fmt.Sprintf("%s: %s", failMsg, stdErr.String())}
	}
	return nil
}

// BuildSidecar builds the sidecar containers list
func BuildSidecar(chaosDetails *types.ChaosDetails) []apiv1.Container {
	var sidecars []apiv1.Container

	for _, c := range chaosDetails.SideCar {
		container := apiv1.Container{
			Name:            c.Name,
			Image:           c.Image,
			ImagePullPolicy: c.ImagePullPolicy,
			Env:             c.ENV,
			EnvFrom:         c.EnvFrom,
		}
		if len(c.Secrets) != 0 {
			var volMounts []apiv1.VolumeMount
			for _, v := range c.Secrets {
				volMounts = append(volMounts, apiv1.VolumeMount{
					Name:      v.Name,
					MountPath: v.MountPath,
				})
			}
			container.VolumeMounts = volMounts
		}
		sidecars = append(sidecars, container)
	}
	return sidecars
}

// GetSidecarVolumes get the list of all the unique volumes from the sidecar
func GetSidecarVolumes(chaosDetails *types.ChaosDetails) []apiv1.Volume {
	var volumes []apiv1.Volume
	k := int32(420)

	secretMap := make(map[string]bool)
	for _, c := range chaosDetails.SideCar {
		if len(c.Secrets) != 0 {
			for _, v := range c.Secrets {
				if _, ok := secretMap[v.Name]; ok {
					continue
				}
				secretMap[v.Name] = true
				volumes = append(volumes, apiv1.Volume{
					Name: v.Name,
					VolumeSource: apiv1.VolumeSource{
						Secret: &apiv1.SecretVolumeSource{
							SecretName:  v.Name,
							DefaultMode: &k,
						},
					},
				})
			}
		}
	}

	return volumes
}

// GetContainerNames gets the name of the main and sidecar containers
func GetContainerNames(chaosDetails *types.ChaosDetails) []string {
	containerNames := []string{chaosDetails.ExperimentName}
	if len(chaosDetails.SideCar) == 0 {
		return containerNames
	}
	for _, c := range chaosDetails.SideCar {
		containerNames = append(containerNames, c.Name)
	}
	return containerNames
}
