package probe

import (
	"fmt"
	"strconv"
	"time"

	"net/http"

	"github.com/kyokomi/emoji"
	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/sirupsen/logrus"
)

// PrepareHTTPProbe contains the steps to prepare the http probe
// http probe can be used to add the probe which will send a request to given url and match the status code
func PrepareHTTPProbe(httpProbes []v1alpha1.HTTPProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {

	if httpProbes != nil {

		for _, probe := range httpProbes {

			// triggering probes on the basis of mode & phase so that probe will only run when they are requested to run
			// if mode is SOT & phase is PreChaos, it will trigger Probes in PreChaos section
			// if mode is EOT & phase is PostChaos, it will trigger Probes in PostChaos section
			// if mode is Edge then independent of phase, it will trigger Probes in both Pre/Post Chaos section
			if (probe.Mode == "SOT" && phase == "PreChaos") || (probe.Mode == "EOT" && phase == "PostChaos") || probe.Mode == "Edge" {

				//DISPLAY THE K8S PROBE INFO
				log.InfoWithValues("[Probe]: The http probe information is as follows", logrus.Fields{
					"Name":                     probe.Name,
					"URL":                      probe.Inputs.URL,
					"Expecected Response Code": probe.Inputs.ExpectedResponseCode,
					"Run Properties":           probe.RunProperties,
					"Mode":                     probe.Mode,
				})

				// trigger the http probe
				err = TriggerHTTPProbe(probe)

				// failing the probe, if the success condition doesn't met after the retry & timeout combinations
				if err != nil {
					log.ErrorWithValues("[Probe]: http probe has been Failed "+emoji.Sprint(":cry:"), logrus.Fields{
						"ProbeName":     probe.Name,
						"ProbeType":     "HTTPProbe",
						"ProbeInstance": phase,
						"ProbeStatus":   "Failed",
					})
					SetProbeVerdictAfterFailure(resultDetails)
					return err
				}
				// counting the passed probes count to generate the score and mark the verdict as passed
				// for edge, probe is marked as Passed if passed in both pre/post chaos checks
				if !(probe.Mode == "Edge" && phase == "PreChaos") {
					resultDetails.PassedProbeCount++
				}
				SetProbeVerdict(resultDetails, "Passed", probe.Name, "HTTPProbe", probe.Mode, phase)
				log.InfoWithValues("[Probe]: http probe has been Passed "+emoji.Sprint(":smile:"), logrus.Fields{
					"ProbeName":     probe.Name,
					"ProbeType":     "HTTPProbe",
					"ProbeInstance": phase,
					"ProbeStatus":   "Passed",
				})
			}
		}
	}
	return nil
}

// TriggerHTTPProbe run the http probe command
func TriggerHTTPProbe(probe v1alpha1.HTTPProbeAttributes) error {

	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	err = retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			// getting the response from the given url
			resp, err := http.Get(probe.Inputs.URL)
			if err != nil {
				return err
			}
			code, _ := strconv.Atoi(probe.Inputs.ExpectedResponseCode)
			// matching the status code w/ expected code
			if resp.StatusCode != code {
				return fmt.Errorf("The response status code doesn't match with expected status code, code: %v", resp.StatusCode)
			}

			return nil
		})
	return err
}
