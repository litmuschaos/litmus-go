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
// http probe can be used to add the probe which will curl an url and match the output
func PrepareHTTPProbe(httpProbes []v1alpha1.HTTPProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {

	if httpProbes != nil {

		for _, probe := range httpProbes {

			//DISPLAY THE K8S PROBE INFO
			log.InfoWithValues("[Probe]: The http probe informations are as follows", logrus.Fields{
				"Name":                     probe.Name,
				"URL":                      probe.Inputs.URL,
				"Expecected Response Code": probe.Inputs.ExpectedResponseCode,
				"Run Properties":           probe.RunProperties,
			})

			// trigger the http probe and storing the output into the out buffer
			err = TriggerHTTPProbe(probe)
			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			if err != nil {
				SetProbeVerdictAfterFailure(resultDetails)
				log.Infof("[Probe]: %v probe has been Failed %v", probe.Name, emoji.Sprint(":cry:"))
				return err
			}
			resultDetails.ProbeCount++
			SetProbeVerdict(resultDetails, "Passed", probe.Name, "HTTPProbe", "edge", phase)
			log.Infof("[Probe]: The %v probe has been Passed %v", probe.Name, emoji.Sprint(":smile:"))
			resultDetails.PassedProbe = append(resultDetails.PassedProbe, probe.Name+"-"+phase)
		}
	}
	return nil
}

// TriggerHTTPProbe run the http probe command and storing the output into the out buffer
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
				return fmt.Errorf("not getting 200 status code, code: %v", resp.StatusCode)
			}

			return nil
		})
	return err
}
