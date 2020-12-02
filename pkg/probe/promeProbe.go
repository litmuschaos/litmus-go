package probe

import (
	"fmt"
	"strings"
	"time"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/types"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// PreparePromeProbe contains the steps to prepare the prometheus probe
// which compares the metrices output exposed at the given endpoint
func PreparePromeProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {

	switch phase {
	case "PreChaos":
		if err := PreChaosPromeProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "PostChaos":
		if err := PostChaosPromeProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "DuringChaos":
		if err := OnChaosPromeProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	default:
		return fmt.Errorf("phase '%s' not supported in the prom probe", phase)
	}
	return nil
}

//PreChaosPromeProbe trigger the prometheus probe for prechaos phase
func PreChaosPromeProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

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

		execCommandDetails, err := CreateHelperPod(probe, resultDetails, clients, chaosDetails, resultDetails.PromProbeImage)
		if err != nil {
			return err
		}

		// triggering the prom probe and storing the output into the out buffer
		err = TriggerPromProbe(probe, execCommandDetails, clients, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PreChaos"); err != nil {
			return err
		}

		// get runId
		runID := GetRunIDFromProbe(resultDetails, probe.Name, probe.Type)

		// deleting the external pod which was created for prom probe
		if err = DeleteProbePod(chaosDetails, clients, runID); err != nil {
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

		execCommandDetails, err := CreateHelperPod(probe, resultDetails, clients, chaosDetails, resultDetails.PromProbeImage)
		if err != nil {
			return err
		}

		// trigger the continuous cmd probe
		go TriggerContinuousPromProbe(probe, execCommandDetails, clients, resultDetails)
	}

	return nil
}

//PostChaosPromeProbe trigger the prometheus probe for postchaos phase
func PostChaosPromeProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

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

		execCommandDetails, err := CreateHelperPod(probe, resultDetails, clients, chaosDetails, resultDetails.PromProbeImage)
		if err != nil {
			return err
		}

		// triggering the prom probe and storing the output into the out buffer
		err = TriggerPromProbe(probe, execCommandDetails, clients, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PostChaos"); err != nil {
			return err
		}

		// get runId
		runID := GetRunIDFromProbe(resultDetails, probe.Name, probe.Type)

		// deleting the external pod which was created for prom probe
		if err = DeleteProbePod(chaosDetails, clients, runID); err != nil {
			return err
		}

	case "Continuous", "OnChaos":

		// it will check for the error, It will detect the error if any error encountered in probe during chaos
		err = CheckForErrorInContinuousProbe(resultDetails, probe.Name)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PostChaos"); err != nil {
			return err
		}
		// get runId
		runID := GetRunIDFromProbe(resultDetails, probe.Name, probe.Type)
		// deleting the external pod, which was created for prom probe
		if err = DeleteProbePod(chaosDetails, clients, runID); err != nil {
			return err
		}
	}

	return nil
}

//OnChaosPromeProbe trigger the prome probe for DuringChaos phase
func OnChaosPromeProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

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

		execCommandDetails, err := CreateHelperPod(probe, resultDetails, clients, chaosDetails, resultDetails.PromProbeImage)
		if err != nil {
			return err
		}

		// trigger the continuous prom probe
		go TriggerOnChaosPromProbe(probe, execCommandDetails, clients, resultDetails, chaosDetails.ChaosDuration)

	}
	return nil
}

// TriggerPromProbe trigger the prometheus probe inside the external pod
func TriggerPromProbe(probe v1alpha1.ProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, resultDetails *types.ResultDetails) error {

	// running the prome probe command and matching the output
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	err = retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			command := []string{"/bin/sh", "-c", "promql --host " + probe.PromProbeInputs.Endpoint + " \"" + probe.PromProbeInputs.Query + "\"" + " --no-headers --output csv"}
			// exec inside the external pod to get the o/p of given command
			output, err := litmusexec.Exec(&execCommandDetails, clients, command)
			if err != nil {
				return errors.Errorf("Unable to get output of cmd command, err: %v", err)
			}

			// extract the values from the metrics
			value, err := ExtractValueFromMetrics(strings.TrimSpace(output))
			if err != nil {
				return err
			}

			if err = ValidateResult(probe.PromProbeInputs.Comparator, value); err != nil {
				log.Warnf("The %v prom probe has been Failed", probe.Name)
				return err
			}

			return nil
		})
	return err
}

// TriggerContinuousPromProbe trigger the continuous prometheus probe
func TriggerContinuousPromProbe(probe v1alpha1.ProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, chaosresult *types.ResultDetails) {

	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
	}

	// it trigger the prom probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
	for {
		err = TriggerPromProbe(probe, execCommandDetails, clients, chaosresult)
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

// TriggerOnChaosPromProbe trigger the onchaos prom probe
func TriggerOnChaosPromProbe(probe v1alpha1.ProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, chaosresult *types.ResultDetails, duration int) {

	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		duration = math.Maximum(0, duration-probe.RunProperties.InitialDelaySeconds)
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(duration) * time.Second

	// it trigger the prome probe for the entire duration of chaos and it fails, if any err encounter
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
			if err = TriggerPromProbe(probe, execCommandDetails, clients, chaosresult); err != nil {
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
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

	rows := strings.Split(metrics, "\n")

	if len(rows) > 1 {
		return "", errors.Errorf("metrics values are greater than one")
	}

	value := strings.Split(rows[0], ",")
	if value[2] == "" {
		return "", errors.Errorf("error while parsing value from derived matrics")
	}

	return value[2], nil
}
