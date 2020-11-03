package probe

import (
	"fmt"
	"strconv"
	"time"

	"net/http"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/sirupsen/logrus"
)

// PrepareHTTPProbe contains the steps to prepare the http probe
// http probe can be used to add the probe which will send a request to given url and match the status code
func PrepareHTTPProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {

	switch phase {
	case "PreChaos":
		if err := PreChaosHTTPProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "PostChaos":
		if err := PostChaosHTTPProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	default:
		return fmt.Errorf("phase '%s' not supported in the http probe", phase)
	}
	return nil
}

// TriggerHTTPProbe run the http probe command
func TriggerHTTPProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails) error {

	// It parse the templated url and return normal string
	// if command doesn't have template, it will return the same command
	probe.HTTPProbeInputs.URL, err = ParseCommand(probe.HTTPProbeInputs.URL, resultDetails)
	if err != nil {
		return err
	}

	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	err = retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			// getting the response from the given url
			resp, err := http.Get(probe.HTTPProbeInputs.URL)
			if err != nil {
				return err
			}
			code, _ := strconv.Atoi(probe.HTTPProbeInputs.ExpectedResponseCode)
			// matching the status code w/ expected code
			if resp.StatusCode != code {
				return fmt.Errorf("The response status code doesn't match with expected status code, code: %v", resp.StatusCode)
			}

			return nil
		})
	return err
}

// TriggerContinuousHTTPProbe trigger the continuous http probes
func TriggerContinuousHTTPProbe(probe v1alpha1.ProbeAttributes, chaosresult *types.ResultDetails) {
	// it trigger the http probe for the entire duration of chaos and it fails, if any error encounter
	// it marked the error for the probes, if any
	for {
		err = TriggerHTTPProbe(probe, chaosresult)
		// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
		if err != nil {
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
					break
				}

			}
			break
		}

		// waiting for the probe polling interval
		time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
	}

}

//PreChaosHTTPProbe trigger the http probe for prechaos phase
func PreChaosHTTPProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch probe.Mode {
	case "SOT", "Edge":

		//DISPLAY THE HTTP PROBE INFO
		log.InfoWithValues("[Probe]: The http probe information is as follows", logrus.Fields{
			"Name":                     probe.Name,
			"URL":                      probe.HTTPProbeInputs.URL,
			"Expecected Response Code": probe.HTTPProbeInputs.ExpectedResponseCode,
			"Run Properties":           probe.RunProperties,
			"Mode":                     probe.Mode,
			"Phase":                    "PreChaos",
		})
		// trigger the http probe
		err = TriggerHTTPProbe(probe, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PreChaos"); err != nil {
			return err
		}
	case "Continuous":

		//DISPLAY THE HTTP PROBE INFO
		log.InfoWithValues("[Probe]: The http probe information is as follows", logrus.Fields{
			"Name":                     probe.Name,
			"URL":                      probe.HTTPProbeInputs.URL,
			"Expecected Response Code": probe.HTTPProbeInputs.ExpectedResponseCode,
			"Run Properties":           probe.RunProperties,
			"Mode":                     probe.Mode,
			"Phase":                    "PreChaos",
		})
		go TriggerContinuousHTTPProbe(probe, resultDetails)
	}
	return nil
}

//PostChaosHTTPProbe trigger the http probe for postchaos phase
func PostChaosHTTPProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch probe.Mode {
	case "EOT", "Edge":

		//DISPLAY THE HTTP PROBE INFO
		log.InfoWithValues("[Probe]: The http probe information is as follows", logrus.Fields{
			"Name":                     probe.Name,
			"URL":                      probe.HTTPProbeInputs.URL,
			"Expecected Response Code": probe.HTTPProbeInputs.ExpectedResponseCode,
			"Run Properties":           probe.RunProperties,
			"Mode":                     probe.Mode,
			"Phase":                    "PostChaos",
		})
		// trigger the http probe
		err = TriggerHTTPProbe(probe, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PostChaos"); err != nil {
			return err
		}
	case "Continuous":
		// it will check for the error, It will detect the error if any error encountered in probe during chaos
		err = CheckForErrorInContinuousProbe(resultDetails, probe.Name)
		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PostChaos"); err != nil {
			return err
		}
	}
	return nil
}
