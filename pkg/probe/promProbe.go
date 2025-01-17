package probe

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	cmp "github.com/litmuschaos/litmus-go/pkg/probe/comparator"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/sirupsen/logrus"
)

// preparePromProbe contains the steps to prepare the prometheus probe
// which compares the metrics output exposed at the given endpoint
func preparePromProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string) error {

	switch strings.ToLower(phase) {
	case "prechaos":
		if err := preChaosPromProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "postchaos":
		if err := postChaosPromProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "duringchaos":
		if err := onChaosPromProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypePromProbe, Target: fmt.Sprintf("{name: %v}", probe.Name), Reason: fmt.Sprintf("phase '%s' not supported in the prom probe", phase)}
	}
	return nil
}

// preChaosPromProbe trigger the prometheus probe for prechaos phase
func preChaosPromProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {
	probeTimeout := getProbeTimeouts(probe.Name, resultDetails.ProbeDetails)

	switch strings.ToLower(probe.Mode) {
	case "sot", "edge":

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
		if probeTimeout.InitialDelay != 0 {
			log.Infof("[Wait]: Waiting for %v before probe execution", probe.RunProperties.InitialDelay)
			time.Sleep(probeTimeout.InitialDelay)
		}

		// triggering the prom probe and storing the output into the out buffer
		if err = triggerPromProbe(probe, resultDetails); err != nil && cerrors.GetErrorType(err) != cerrors.FailureTypePromProbe {
			return err
		}

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = markedVerdictInEnd(err, resultDetails, probe, "PreChaos"); err != nil {
			return err
		}

	case "continuous":

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
		go triggerContinuousPromProbe(probe, clients, resultDetails, chaosDetails)
	}

	return nil
}

// postChaosPromProbe trigger the prometheus probe for postchaos phase
func postChaosPromProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {
	probeTimeout := getProbeTimeouts(probe.Name, resultDetails.ProbeDetails)

	switch strings.ToLower(probe.Mode) {
	case "eot", "edge":

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
		if probeTimeout.InitialDelay != 0 {
			log.Infof("[Wait]: Waiting for %v before probe execution", probe.RunProperties.InitialDelay)
			time.Sleep(probeTimeout.InitialDelay)
		}

		// triggering the prom probe and storing the output into the out buffer
		if err = triggerPromProbe(probe, resultDetails); err != nil && cerrors.GetErrorType(err) != cerrors.FailureTypePromProbe {
			return err
		}

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = markedVerdictInEnd(err, resultDetails, probe, "PostChaos"); err != nil {
			return err
		}

	case "continuous", "onchaos":

		// it will check for the error, It will detect the error if any error encountered in probe during chaos
		if err = checkForErrorInContinuousProbe(resultDetails, probe.Name, chaosDetails.Delay, chaosDetails.Timeout); err != nil && cerrors.GetErrorType(err) != cerrors.FailureTypePromProbe && cerrors.GetErrorType(err) != cerrors.FailureTypeProbeTimeout {
			return err
		}

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		if err = markedVerdictInEnd(err, resultDetails, probe, "PostChaos"); err != nil {
			return err
		}

	}
	return nil
}

// onChaosPromProbe trigger the prom probe for DuringChaos phase
func onChaosPromProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch strings.ToLower(probe.Mode) {
	case "onchaos":

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
		go triggerOnChaosPromProbe(probe, clients, resultDetails, chaosDetails)
	}
	return nil
}

// triggerPromProbe trigger the prometheus probe inside the external pod
func triggerPromProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails) error {
	probeTimeout := getProbeTimeouts(probe.Name, resultDetails.ProbeDetails)

	var description string
	// running the prom probe command and matching the output
	// it will retry for some retry count, in each iteration of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the interval and again execute the command until timeout expires
	if err := retry.Times(uint(getAttempts(probe.RunProperties.Attempt, probe.RunProperties.Retry))).
		Timeout(probeTimeout.ProbeTimeout).
		Wait(probeTimeout.Interval).
		TryWithTimeout(func(attempt uint) error {
			var command string
			// It will use query or queryPath to get the prometheus metrics
			// if both are provided, it will use query
			if probe.PromProbeInputs.Query != "" {
				command = "promql --host " + probe.PromProbeInputs.Endpoint + " \"" + probe.PromProbeInputs.Query + "\"" + " --output csv"
			} else if probe.PromProbeInputs.QueryPath != "" {
				command = "promql --host " + probe.PromProbeInputs.Endpoint + " \"$(cat " + probe.PromProbeInputs.QueryPath + ")\"" + " --output csv"
			} else {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypePromProbe, Target: fmt.Sprintf("{name: %v}", probe.Name), Reason: "[Probe]: Any one of query or queryPath is required"}
			}

			var out, errOut bytes.Buffer
			// run the inline command probe
			cmd := exec.Command("/bin/sh", "-c", command)
			cmd.Stdout = &out
			cmd.Stderr = &errOut
			if err := cmd.Run(); err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypePromProbe, Target: fmt.Sprintf("{name: %v}", probe.Name), Reason: fmt.Sprintf("unable to run command, error: %s", errOut.String())}
			}

			// extract the values from the metrics
			value, err := extractValueFromMetrics(strings.TrimSpace(out.String()), probe.Name)
			if err != nil {
				return err
			}

			rc := getAndIncrementRunCount(resultDetails, probe.Name)
			// comparing the metrics output with the expected criteria
			if err = cmp.RunCount(rc).
				FirstValue(value).
				SecondValue(probe.PromProbeInputs.Comparator.Value).
				Criteria(probe.PromProbeInputs.Comparator.Criteria).
				ProbeName(probe.Name).
				ProbeVerbosity(probe.RunProperties.Verbosity).
				CompareFloat(cerrors.FailureTypePromProbe); err != nil {
				log.Errorf("The %v prom probe has been Failed, err: %v", probe.Name, err)
				return err
			}
			description = fmt.Sprintf("Obtained the specified prometheus metrics. Actual value: %s. Expected value: %s", value, probe.PromProbeInputs.Comparator.Value)
			return nil
		}); err != nil {
		return checkProbeTimeoutError(probe.Name, cerrors.FailureTypePromProbe, err)
	}
	setProbeDescription(resultDetails, probe, description)
	return nil
}

// triggerContinuousPromProbe trigger the continuous prometheus probe
func triggerContinuousPromProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) {
	probeTimeout := getProbeTimeouts(probe.Name, chaosresult.ProbeDetails)

	var isExperimentFailed bool
	// waiting for initial delay
	if probeTimeout.InitialDelay != 0 {
		log.Infof("[Wait]: Waiting for %v before probe execution", probe.RunProperties.InitialDelay)
		time.Sleep(probeTimeout.InitialDelay)
	}

	// it trigger the prom probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
loop:
	for {
		select {
		case <-chaosDetails.ProbeContext.Ctx.Done():
			log.Infof("Stopping %s continuous Probe", probe.Name)
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].HasProbeCompleted = true
				}
			}
			break loop
		default:
			err = triggerPromProbe(probe, chaosresult)
			// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
			if err != nil {
				err = addProbePhase(err, string(chaosDetails.Phase))
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
						chaosresult.ProbeDetails[index].HasProbeCompleted = true
						chaosresult.ProbeDetails[index].Status.Description = getDescription(err)
						log.Errorf("The %v prom probe has been Failed, err: %v", probe.Name, err)
						isExperimentFailed = true
						break loop
					}
				}
			}
			// waiting for the probe polling interval
			time.Sleep(probeTimeout.ProbePollingInterval)
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

// triggerOnChaosPromProbe trigger the onchaos prom probe
func triggerOnChaosPromProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) {
	probeTimeout := getProbeTimeouts(probe.Name, chaosresult.ProbeDetails)

	var isExperimentFailed bool
	duration := chaosDetails.ChaosDuration
	// waiting for initial delay
	if probeTimeout.InitialDelay != 0 {
		log.Infof("[Wait]: Waiting for %v before probe execution", probe.RunProperties.InitialDelay)
		time.Sleep(probeTimeout.InitialDelay)
		duration = math.Maximum(0, duration-int(probeTimeout.InitialDelay))
	}

	endTime := time.After(time.Duration(duration) * time.Second)

	// it trigger the prom probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
loop:
	for {
		select {
		case <-endTime:
			log.Infof("[Chaos]: Time is up for the %v probe", probe.Name)
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].HasProbeCompleted = true
				}
			}
			endTime = nil
			break loop
		default:
			// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
			if err = triggerPromProbe(probe, chaosresult); err != nil {
				err = addProbePhase(err, string(chaosDetails.Phase))
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
						chaosresult.ProbeDetails[index].HasProbeCompleted = true
						chaosresult.ProbeDetails[index].Status.Description = getDescription(err)
						log.Errorf("The %v prom probe has been Failed, err: %v", probe.Name, err)
						isExperimentFailed = true
						break loop
					}
				}
			}

			select {
			case <-chaosDetails.ProbeContext.Ctx.Done():
				log.Infof("Stopping %s continuous Probe", probe.Name)
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].HasProbeCompleted = true
					}
				}
				break loop
			default:
				// waiting for the probe polling interval
				time.Sleep(probeTimeout.ProbePollingInterval)
			}
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

// extractValueFromMetrics extract the value field from the prometheus metrix
func extractValueFromMetrics(metrics, probeName string) (string, error) {

	// spliting the metrics based on newline as metrics may have multiple entries
	rows := strings.Split(metrics, "\n")

	// output should contains exact one metrics entry along with header
	// erroring out the cases where it contains more or less entries
	if len(rows) > 2 {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypePromProbe, Target: fmt.Sprintf("{name: %v}", probeName), Reason: "metrics entries can't be more than two"}
	} else if len(rows) < 2 {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypePromProbe, Target: fmt.Sprintf("{name: %v}", probeName), Reason: "metrics doesn't contains required values"}
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
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypePromProbe, Target: fmt.Sprintf("{name: %v}", probeName), Reason: "metrics entries doesn't contains value column"}
	}

	// splitting the metrics entries which are available as comma separated
	values := strings.Split(rows[1], ",")
	if values[indexForValueColumn] == "" {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypePromProbe, Target: fmt.Sprintf("{name: %v}", probeName), Reason: "error while parsing value from derived matrics"}
	}
	return values[indexForValueColumn], nil
}
