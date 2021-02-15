package probe

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// PreparePromProbe contains the steps to prepare the prometheus probe
// which compares the metrices output exposed at the given endpoint
func PreparePromProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {

	switch phase {
	case "PreChaos":
		if err := PreChaosPromProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "PostChaos":
		if err := PostChaosPromProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "DuringChaos":
		if err := OnChaosPromProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	default:
		return fmt.Errorf("phase '%s' not supported in the prom probe", phase)
	}
	return nil
}

//PreChaosPromProbe trigger the prometheus probe for prechaos phase
func PreChaosPromProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch probe.Mode {
	case "SOT", "Edge":

		//DISPLAY THE PROMETHEUS PROBE INFO
		log.InfoWithValues("[Probe]: The prometheus probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Query":          probe.PromProbeInputs.Query,
			"Endpoint":       probe.PromProbeInputs.Endpoint,
			"Comparator":     probe.PromProbeInputs.Comparator,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PreChaos",
		})

		// waiting for initial delay
		if probe.RunProperties.InitialDelaySeconds != 0 {
			log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
			time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		}

		// triggering the prom probe and storing the output into the out buffer
		err = TriggerPromProbe(probe, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PreChaos"); err != nil {
			return err
		}

	case "Continuous":

		//DISPLAY THE PROMETHEUS PROBE INFO
		log.InfoWithValues("[Probe]: The prometheus probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Query":          probe.PromProbeInputs.Query,
			"Endpoint":       probe.PromProbeInputs.Endpoint,
			"Comparator":     probe.PromProbeInputs.Comparator,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PreChaos",
		})

		// trigger the continuous cmd probe
		go TriggerContinuousPromProbe(probe, resultDetails)
	}

	return nil
}

//PostChaosPromProbe trigger the prometheus probe for postchaos phase
func PostChaosPromProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch probe.Mode {
	case "EOT", "Edge":

		//DISPLAY THE PROMETHEUS PROBE INFO
		log.InfoWithValues("[Probe]: The prometheus probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Query":          probe.PromProbeInputs.Query,
			"Endpoint":       probe.PromProbeInputs.Endpoint,
			"Comparator":     probe.PromProbeInputs.Comparator,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PostChaos",
		})

		// waiting for initial delay
		if probe.RunProperties.InitialDelaySeconds != 0 {
			log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
			time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		}

		// triggering the prom probe and storing the output into the out buffer
		err = TriggerPromProbe(probe, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PostChaos"); err != nil {
			return err
		}

	case "Continuous", "OnChaos":

		// it will check for the error, It will detect the error if any error encountered in probe during chaos
		err = CheckForErrorInContinuousProbe(resultDetails, probe.Name)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PostChaos"); err != nil {
			return err
		}

	}
	return nil
}

//OnChaosPromProbe trigger the prom probe for DuringChaos phase
func OnChaosPromProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch probe.Mode {
	case "OnChaos":

		//DISPLAY THE PROMETHEUS PROBE INFO
		log.InfoWithValues("[Probe]: The prometheus probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Query":          probe.PromProbeInputs.Query,
			"Endpoint":       probe.PromProbeInputs.Endpoint,
			"Comparator":     probe.PromProbeInputs.Comparator,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "DuringChaos",
		})

		// trigger the continuous prom probe
		go TriggerOnChaosPromProbe(probe, resultDetails, chaosDetails.ChaosDuration)
	}
	return nil
}

// TriggerPromProbe trigger the prometheus probe inside the external pod
func TriggerPromProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails) error {

	// running the prom probe command and matching the output
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the interval and again execute the command until timeout expires
	err = retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			var command string
			// It will use query or queryPath to get the prometheus metrics
			// if both are provided, it will use query
			if probe.PromProbeInputs.Query != "" {
				command = "promql --host " + probe.PromProbeInputs.Endpoint + " \"" + probe.PromProbeInputs.Query + "\"" + " --output csv"
			} else if probe.PromProbeInputs.QueryPath != "" {
				command = "promql --host " + probe.PromProbeInputs.Endpoint + " \"$(cat " + probe.PromProbeInputs.QueryPath + ")\"" + " --output csv"
			} else {
				return errors.Errorf("[Probe]: Any one of query or queryPath is required")
			}

			var out, errOut bytes.Buffer
			// run the inline command probe
			cmd := exec.Command("/bin/sh", "-c", command)
			cmd.Stdout = &out
			cmd.Stderr = &errOut
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("Unable to run command, err: %v; error output: %v", err, errOut.String())
			}

			// extract the values from the metrics
			value, err := ExtractValueFromMetrics(strings.TrimSpace(out.String()))
			if err != nil {
				return err
			}

			// comparing the metrics output with the expected criteria
			if err = FirstValue(probe.PromProbeInputs.Comparator.Value).
				SecondValue(value).
				Criteria(probe.PromProbeInputs.Comparator.Criteria).
				CompareFloat(); err != nil {
				log.Errorf("The %v prom probe has been Failed, err: %v", probe.Name, err)
				return err
			}
			return nil
		})
	return err
}

// TriggerContinuousPromProbe trigger the continuous prometheus probe
func TriggerContinuousPromProbe(probe v1alpha1.ProbeAttributes, chaosresult *types.ResultDetails) {

	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
	}

	// it trigger the prom probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
loop:
	for {
		err = TriggerPromProbe(probe, chaosresult)
		// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
		if err != nil {
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
					log.Errorf("The %v prom probe has been Failed, err: %v", probe.Name, err)
					break loop
				}
			}
		}
		// waiting for the probe polling interval
		time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
	}
}

// TriggerOnChaosPromProbe trigger the onchaos prom probe
func TriggerOnChaosPromProbe(probe v1alpha1.ProbeAttributes, chaosresult *types.ResultDetails, duration int) {

	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		duration = math.Maximum(0, duration-probe.RunProperties.InitialDelaySeconds)
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(duration) * time.Second

	// it trigger the prom probe for the entire duration of chaos and it fails, if any err encounter
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
			// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
			if err = TriggerPromProbe(probe, chaosresult); err != nil {
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
						log.Errorf("The %v prom probe has been Failed, err: %v", probe.Name, err)
						break loop
					}
				}
			}
			// waiting for the probe polling interval
			time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
		}
	}
}

// ExtractValueFromMetrics extract the value field from the prometheus metrix
func ExtractValueFromMetrics(metrics string) (string, error) {

	// spliting the metrics based on newline as metrics may have multiple entries
	rows := strings.Split(metrics, "\n")

	// output should contains exact one metrics entry along with header
	// erroring out the cases where it contains more or less entries
	if len(rows) > 2 {
		return "", errors.Errorf("metrics entries can't be more than two")
	} else if len(rows) < 2 {
		return "", errors.Errorf("metrics doesn't contains required values")
	}

	// deriving the index for the value column from the headers
	headerColumn := strings.Split(rows[0], ",")
	indexForValueColumn := -1
	for index := range headerColumn {
		if strings.ToLower(headerColumn[index]) == "value" {
			indexForValueColumn = index
			break
		}
	}
	if indexForValueColumn == -1 {
		return "", errors.Errorf("metrics entries doesn't contains value column")
	}

	// splitting the metrics entries which are availble as comma separated
	values := strings.Split(rows[1], ",")
	if values[indexForValueColumn] == "" {
		return "", errors.Errorf("error while parsing value from derived matrics")
	}
	return values[indexForValueColumn], nil
}
