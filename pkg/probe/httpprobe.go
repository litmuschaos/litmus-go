package probe

import (
	"bytes"
	"fmt"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	"crypto/tls"
	"net/http"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	cmp "github.com/litmuschaos/litmus-go/pkg/probe/comparator"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// prepareHTTPProbe contains the steps to prepare the http probe
// http probe can be used to add the probe which will send a request to given url and match the status code
func prepareHTTPProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string) error {

	switch strings.ToLower(phase) {
	case "prechaos":
		if err := preChaosHTTPProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "postchaos":
		if err := postChaosHTTPProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "duringchaos":
		onChaosHTTPProbe(probe, resultDetails, clients, chaosDetails)
	default:
		return errors.Errorf("phase '%s' not supported in the http probe", phase)
	}
	return nil
}

// triggerHTTPProbe run the http probe command
func triggerHTTPProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails) error {

	// It parse the templated url and return normal string
	// if command doesn't have template, it will return the same command
	probe.HTTPProbeInputs.URL, err = parseCommand(probe.HTTPProbeInputs.URL, resultDetails)
	if err != nil {
		return err
	}

	// it fetch the http method type
	method := getHTTPMethodType(probe.HTTPProbeInputs.Method)

	// initialize simple http client with default attributes
	timeout := time.Duration(probe.HTTPProbeInputs.ResponseTimeout) * time.Millisecond
	client := &http.Client{Timeout: timeout}
	// impose properties to http client with cert check disabled
	if probe.HTTPProbeInputs.InsecureSkipVerify {
		transCfg := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{Transport: transCfg, Timeout: timeout}
	}

	switch method {
	case "Get":
		log.InfoWithValues("[Probe]: HTTP get method informations", logrus.Fields{
			"Name":            probe.Name,
			"URL":             probe.HTTPProbeInputs.URL,
			"Criteria":        probe.HTTPProbeInputs.Method.Get.Criteria,
			"ResponseCode":    probe.HTTPProbeInputs.Method.Get.ResponseCode,
			"ResponseTimeout": probe.HTTPProbeInputs.ResponseTimeout,
		})
		if err := httpGet(probe, client, resultDetails); err != nil {
			return err
		}
	case "Post":
		log.InfoWithValues("[Probe]: HTTP Post method informations", logrus.Fields{
			"Name":            probe.Name,
			"URL":             probe.HTTPProbeInputs.URL,
			"Body":            probe.HTTPProbeInputs.Method.Post.Body,
			"BodyPath":        probe.HTTPProbeInputs.Method.Post.BodyPath,
			"ContentType":     probe.HTTPProbeInputs.Method.Post.ContentType,
			"ResponseTimeout": probe.HTTPProbeInputs.ResponseTimeout,
		})
		if err := httpPost(probe, client, resultDetails); err != nil {
			return err
		}
	}
	return nil
}

// it fetch the http method type
// it supports Get and Post methods
func getHTTPMethodType(httpMethod v1alpha1.HTTPMethod) string {
	if !reflect.DeepEqual(httpMethod.Get, v1alpha1.GetMethod{}) {
		return "Get"
	}
	return "Post"
}

// httpGet send the http Get request to the given URL and verify the response code to follow the specified criteria
func httpGet(probe v1alpha1.ProbeAttributes, client *http.Client, resultDetails *types.ResultDetails) error {
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the interval and again execute the command until timeout expires
	return retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			// getting the response from the given url
			resp, err := client.Get(probe.HTTPProbeInputs.URL)
			if err != nil {
				return err
			}

			code := strconv.Itoa(resp.StatusCode)
			rc := getAndIncrementRunCount(resultDetails, probe.Name)

			// comparing the response code with the expected criteria
			if err = cmp.RunCount(rc).
				FirstValue(code).
				SecondValue(probe.HTTPProbeInputs.Method.Get.ResponseCode).
				Criteria(probe.HTTPProbeInputs.Method.Get.Criteria).
				CompareInt(); err != nil {
				log.Errorf("The %v http probe get method has Failed, err: %v", probe.Name, err)
				return err
			}
			return nil
		})
}

// httpPost send the http post request to the given URL
func httpPost(probe v1alpha1.ProbeAttributes, client *http.Client, resultDetails *types.ResultDetails) error {
	body, err := getHTTPBody(probe.HTTPProbeInputs.Method.Post)
	if err != nil {
		return err
	}
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the interval and again execute the command until timeout expires
	return retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			resp, err := client.Post(probe.HTTPProbeInputs.URL, probe.HTTPProbeInputs.Method.Post.ContentType, strings.NewReader(body))
			if err != nil {
				return err
			}
			code := strconv.Itoa(resp.StatusCode)
			rc := getAndIncrementRunCount(resultDetails, probe.Name)

			// comparing the response code with the expected criteria
			if err = cmp.RunCount(rc).
				FirstValue(code).
				SecondValue(probe.HTTPProbeInputs.Method.Post.ResponseCode).
				Criteria(probe.HTTPProbeInputs.Method.Post.Criteria).
				CompareInt(); err != nil {
				log.Errorf("The %v http probe post method has Failed, err: %v", probe.Name, err)
				return err
			}
			return nil
		})
}

// getHTTPBody fetch the http body for the post request
// It will use body or bodyPath attributes to get the http request body
// if both are provided, it will use body field
func getHTTPBody(httpBody v1alpha1.PostMethod) (string, error) {

	if httpBody.Body != "" {
		return httpBody.Body, nil
	}

	var command string

	if httpBody.BodyPath != "" {
		command = "cat " + httpBody.BodyPath
	} else {
		return "", errors.Errorf("[Probe]: Any one of body or bodyPath is required")
	}

	var out, errOut bytes.Buffer
	// run the inline command probe
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("unable to run command, err: %v; error output: %v", err, errOut.String())
	}
	return out.String(), nil
}

// triggerContinuousHTTPProbe trigger the continuous http probes
func triggerContinuousHTTPProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) {
	var isExperimentFailed bool
	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
	}

	// it trigger the http probe for the entire duration of chaos and it fails, if any error encounter
	// it marked the error for the probes, if any
loop:
	for {
		err = triggerHTTPProbe(probe, chaosresult)
		// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
		if err != nil {
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
					log.Errorf("The %v http probe has been Failed, err: %v", probe.Name, err)
					isExperimentFailed = true
					break loop
				}
			}
		}
		// waiting for the probe polling interval
		time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
	}
	// if experiment fails and stopOnfailure is provided as true then it will patch the chaosengine for abort
	// if experiment fails but stopOnfailure is provided as false then it will continue the execution
	// and failed the experiment in the end
	if isExperimentFailed && probe.RunProperties.StopOnFailure {
		if err := stopChaosEngine(probe, clients, chaosresult, chaosDetails); err != nil {
			log.Errorf("unable to patch chaosengine to stop, err: %v", err)
		}
	}
}

//preChaosHTTPProbe trigger the http probe for prechaos phase
func preChaosHTTPProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch probe.Mode {
	case "SOT", "Edge":

		//DISPLAY THE HTTP PROBE INFO
		log.InfoWithValues("[Probe]: The http probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"URL":            probe.HTTPProbeInputs.URL,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PreChaos",
		})

		// waiting for initial delay
		if probe.RunProperties.InitialDelaySeconds != 0 {
			log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
			time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		}
		// trigger the http probe
		err = triggerHTTPProbe(probe, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = markedVerdictInEnd(err, resultDetails, probe, "PreChaos"); err != nil {
			return err
		}
	case "Continuous":

		//DISPLAY THE HTTP PROBE INFO
		log.InfoWithValues("[Probe]: The http probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"URL":            probe.HTTPProbeInputs.URL,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PreChaos",
		})
		go triggerContinuousHTTPProbe(probe, clients, resultDetails, chaosDetails)
	}
	return nil
}

//postChaosHTTPProbe trigger the http probe for postchaos phase
func postChaosHTTPProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch probe.Mode {
	case "EOT", "Edge":

		//DISPLAY THE HTTP PROBE INFO
		log.InfoWithValues("[Probe]: The http probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"URL":            probe.HTTPProbeInputs.URL,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PostChaos",
		})

		// waiting for initial delay
		if probe.RunProperties.InitialDelaySeconds != 0 {
			log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
			time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		}

		// trigger the http probe
		err = triggerHTTPProbe(probe, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = markedVerdictInEnd(err, resultDetails, probe, "PostChaos"); err != nil {
			return err
		}
	case "Continuous", "OnChaos":
		// it will check for the error, It will detect the error if any error encountered in probe during chaos
		err = checkForErrorInContinuousProbe(resultDetails, probe.Name)
		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		if err = markedVerdictInEnd(err, resultDetails, probe, "PostChaos"); err != nil {
			return err
		}
	}
	return nil
}

// triggerOnChaosHTTPProbe trigger the onchaos http probes
func triggerOnChaosHTTPProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) {

	var isExperimentFailed bool
	duration := chaosDetails.ChaosDuration
	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		duration = math.Maximum(0, duration-probe.RunProperties.InitialDelaySeconds)
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(duration) * time.Second

	// it trigger the http probe for the entire duration of chaos and it fails, if any error encounter
	// it marked the error for the probes, if any
loop:
	for {
		endTime = time.After(timeDelay)
		select {
		case <-endTime:
			log.Infof("[Chaos]: Time is up for the %v probe", probe.Name)
			endTime = nil
			break loop
		default:
			err = triggerHTTPProbe(probe, chaosresult)
			// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
			if err != nil {
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
						isExperimentFailed = true
						break loop
					}
				}
			}

			// waiting for the probe polling interval
			time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
		}
	}
	// if experiment fails and stopOnfailure is provided as true then it will patch the chaosengine for abort
	// if experiment fails but stopOnfailure is provided as false then it will continue the execution
	// and failed the experiment in the end
	if isExperimentFailed && probe.RunProperties.StopOnFailure {
		if err := stopChaosEngine(probe, clients, chaosresult, chaosDetails); err != nil {
			log.Errorf("unable to patch chaosengine to stop, err: %v", err)
		}
	}
}

//onChaosHTTPProbe trigger the http probe for DuringChaos phase
func onChaosHTTPProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) {

	switch probe.Mode {
	case "OnChaos":

		//DISPLAY THE HTTP PROBE INFO
		log.InfoWithValues("[Probe]: The http probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"URL":            probe.HTTPProbeInputs.URL,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "DuringChaos",
		})
		go triggerOnChaosHTTPProbe(probe, clients, resultDetails, chaosDetails)
	}

}
