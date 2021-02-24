package probe

import (
	"bytes"
	"fmt"
	"math/rand"
	"os/exec"
	"strings"
	"time"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	cmp "github.com/litmuschaos/litmus-go/pkg/probe/comparator"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrepareCmdProbe contains the steps to prepare the cmd probe
// cmd probe can be used to add the command probes
// it can be of two types one: which need a source(an external image)
// another: any inline command which can be run without source image, directly via go-runner image
func PrepareCmdProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {

	switch phase {
	case "PreChaos":
		if err := PreChaosCmdProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "PostChaos":
		if err := PostChaosCmdProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "DuringChaos":
		if err := OnChaosCmdProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	default:
		return fmt.Errorf("phase '%s' not supported in the cmd probe", phase)
	}
	return nil
}

// TriggerInlineCmdProbe trigger the cmd probe and storing the output into the out buffer
func TriggerInlineCmdProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails) error {

	// It parse the templated command and return normal string
	// if command doesn't have template, it will return the same command
	probe.CmdProbeInputs.Command, err = ParseCommand(probe.CmdProbeInputs.Command, resultDetails)
	if err != nil {
		return err
	}

	// running the cmd probe command and storing the output into the out buffer
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	return retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			var out, errOut bytes.Buffer
			// run the inline command probe
			cmd := exec.Command("/bin/sh", "-c", probe.CmdProbeInputs.Command)
			cmd.Stdout = &out
			cmd.Stderr = &errOut
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("Unable to run command, err: %v; error output: %v", err, errOut.String())
			}

			if err = ValidateResult(probe.CmdProbeInputs.Comparator, strings.TrimSpace(out.String())); err != nil {
				log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
				return err
			}

			probes := types.ProbeArtifact{}
			probes.ProbeArtifacts.Register = strings.TrimSpace(out.String())
			resultDetails.ProbeArtifacts[probe.Name] = probes
			return nil
		})
}

// TriggerSourceCmdProbe trigger the cmd probe inside the external pod
func TriggerSourceCmdProbe(probe v1alpha1.ProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, resultDetails *types.ResultDetails) error {

	// It parse the templated command and return normal string
	// if command doesn't have template, it will return the same command
	probe.CmdProbeInputs.Command, err = ParseCommand(probe.CmdProbeInputs.Command, resultDetails)
	if err != nil {
		return err
	}

	// running the cmd probe command and matching the output
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	return retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			command := append([]string{"/bin/sh", "-c"}, probe.CmdProbeInputs.Command)
			// exec inside the external pod to get the o/p of given command
			output, err := litmusexec.Exec(&execCommandDetails, clients, command)
			if err != nil {
				return errors.Errorf("Unable to get output of cmd command, err: %v", err)
			}

			if err = ValidateResult(probe.CmdProbeInputs.Comparator, strings.TrimSpace(output)); err != nil {
				log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
				return err
			}

			probes := types.ProbeArtifact{}
			probes.ProbeArtifacts.Register = strings.TrimSpace(output)
			resultDetails.ProbeArtifacts[probe.Name] = probes
			return nil
		})
}

// CreateProbePod creates an external pod with source image for the cmd probe
func CreateProbePod(clients clients.ClientSets, chaosDetails *types.ChaosDetails, runID, source string) error {
	cmdProbe := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      chaosDetails.ExperimentName + "-probe-" + runID,
			Namespace: chaosDetails.ChaosNamespace,
			Labels: map[string]string{
				"name":     chaosDetails.ExperimentName + "-probe-" + runID,
				"chaosUID": string(chaosDetails.ChaosUID),
			},
		},
		Spec: apiv1.PodSpec{
			RestartPolicy: apiv1.RestartPolicyNever,
			Containers: []apiv1.Container{
				{
					Name:            chaosDetails.ExperimentName + "-probe",
					Image:           source,
					ImagePullPolicy: apiv1.PullPolicy(chaosDetails.ProbeImagePullPolicy),
					Command: []string{
						"/bin/sh",
					},
					Args: []string{
						"-c",
						"sleep 10000",
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).Create(cmdProbe)
	return err

}

//DeleteProbePod deletes the probe pod and wait until it got terminated
func DeleteProbePod(chaosDetails *types.ChaosDetails, clients clients.ClientSets, runID string) error {

	if err := clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).Delete(chaosDetails.ExperimentName+"-probe-"+runID, &v1.DeleteOptions{}); err != nil {
		return err
	}

	// waiting till the termination of the pod
	return retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: chaosDetails.ExperimentName + "-probe-" + runID})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Probe Pod is not deleted yet, err: %v", err)
			}
			return nil
		})
}

// GetRunID generate a random string
func GetRunID() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")
	rand.Seed(time.Now().Unix())
	runID := make([]rune, 6)
	for i := range runID {
		runID[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(runID)
}

// TriggerInlineContinuousCmdProbe trigger the inline continuous cmd probes
func TriggerInlineContinuousCmdProbe(probe v1alpha1.ProbeAttributes, chaosresult *types.ResultDetails) {

	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
	}

	// it trigger the inline cmd probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
loop:
	for {
		err = TriggerInlineCmdProbe(probe, chaosresult)
		// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
		if err != nil {
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
					log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
					break loop
				}
			}
		}
		// waiting for the probe polling interval
		time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
	}
}

// TriggerInlineOnChaosCmdProbe trigger the inline onchaos cmd probes
func TriggerInlineOnChaosCmdProbe(probe v1alpha1.ProbeAttributes, chaosresult *types.ResultDetails, duration int) {

	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		duration = math.Maximum(0, duration-probe.RunProperties.InitialDelaySeconds)
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(duration) * time.Second

	// it trigger the inline cmd probe for the entire duration of chaos and it fails, if any err encounter
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
			err = TriggerInlineCmdProbe(probe, chaosresult)
			// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
			if err != nil {
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
						log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
						break loop
					}
				}
			}
			// waiting for the probe polling interval
			time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
		}
	}
}

// TriggerSourceOnChaosCmdProbe trigger the onchaos cmd probes having need some external source image
func TriggerSourceOnChaosCmdProbe(probe v1alpha1.ProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, chaosresult *types.ResultDetails, duration int) {

	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		duration = math.Maximum(0, duration-probe.RunProperties.InitialDelaySeconds)
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(duration) * time.Second

	// it trigger the cmd probe for the entire duration of chaos and it fails, if any err encounter
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
			if err = TriggerSourceCmdProbe(probe, execCommandDetails, clients, chaosresult); err != nil {
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
						log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
						break loop
					}
				}
			}
			// waiting for the probe polling interval
			time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
		}
	}
}

// TriggerSourceContinuousCmdProbe trigger the continuous cmd probes having need some external source image
func TriggerSourceContinuousCmdProbe(probe v1alpha1.ProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, chaosresult *types.ResultDetails) {

	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
	}

	// it trigger the cmd probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
loop:
	for {
		err = TriggerSourceCmdProbe(probe, execCommandDetails, clients, chaosresult)
		// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
		if err != nil {
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
					log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
					break loop
				}
			}
		}
		// waiting for the probe polling interval
		time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
	}
}

// ValidateResult validate the probe result to specified comparison operation
// it supports int, float, string operands
func ValidateResult(comparator v1alpha1.ComparatorInfo, cmdOutput string) error {
	switch strings.ToLower(comparator.Type) {
	case "int":
		if err = cmp.FirstValue(cmdOutput).
			SecondValue(comparator.Value).
			Criteria(comparator.Criteria).
			CompareInt(); err != nil {
			return err
		}
	case "float":
		if err = cmp.FirstValue(cmdOutput).
			SecondValue(comparator.Value).
			Criteria(comparator.Criteria).
			CompareFloat(); err != nil {
			return err
		}
	case "string":
		if err = cmp.FirstValue(cmdOutput).
			SecondValue(comparator.Value).
			Criteria(comparator.Criteria).
			CompareString(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("comparator type '%s' not supported in the cmd probe", comparator.Type)
	}
	return nil
}

//PreChaosCmdProbe trigger the cmd probe for prechaos phase
func PreChaosCmdProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch probe.Mode {
	case "SOT", "Edge":

		//DISPLAY THE Cmd PROBE INFO
		log.InfoWithValues("[Probe]: The cmd probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Command":        probe.CmdProbeInputs.Command,
			"Comparator":     probe.CmdProbeInputs.Comparator,
			"Source":         probe.CmdProbeInputs.Source,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PreChaos",
		})

		// waiting for initial delay
		if probe.RunProperties.InitialDelaySeconds != 0 {
			log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
			time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		}

		// triggering the cmd probe for the inline mode
		if probe.CmdProbeInputs.Source == "inline" {
			err = TriggerInlineCmdProbe(probe, resultDetails)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			// it will update the status of all the unrun probes as well
			if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PreChaos"); err != nil {
				return err
			}
		} else {

			execCommandDetails, err := CreateHelperPod(probe, resultDetails, clients, chaosDetails, probe.CmdProbeInputs.Source)
			if err != nil {
				return err
			}

			// triggering the cmd probe and storing the output into the out buffer
			err = TriggerSourceCmdProbe(probe, execCommandDetails, clients, resultDetails)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			// it will update the status of all the unrun probes as well
			if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PreChaos"); err != nil {
				return err
			}

			// get runId
			runID := GetRunIDFromProbe(resultDetails, probe.Name, probe.Type)

			// deleting the external pod which was created for cmd probe
			if err = DeleteProbePod(chaosDetails, clients, runID); err != nil {
				return err
			}
		}

	case "Continuous":

		//DISPLAY THE Cmd PROBE INFO
		log.InfoWithValues("[Probe]: The cmd probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Command":        probe.CmdProbeInputs.Command,
			"Comparator":     probe.CmdProbeInputs.Comparator,
			"Source":         probe.CmdProbeInputs.Source,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PreChaos",
		})
		if probe.CmdProbeInputs.Source == "inline" {
			go TriggerInlineContinuousCmdProbe(probe, resultDetails)
		} else {

			execCommandDetails, err := CreateHelperPod(probe, resultDetails, clients, chaosDetails, probe.CmdProbeInputs.Source)
			if err != nil {
				return err
			}

			// trigger the continuous cmd probe
			go TriggerSourceContinuousCmdProbe(probe, execCommandDetails, clients, resultDetails)
		}

	}
	return nil
}

//PostChaosCmdProbe trigger cmd probe for post chaos phase
func PostChaosCmdProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch probe.Mode {
	case "EOT", "Edge":

		//DISPLAY THE Cmd PROBE INFO
		log.InfoWithValues("[Probe]: The cmd probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Command":        probe.CmdProbeInputs.Command,
			"Comparator":     probe.CmdProbeInputs.Comparator,
			"Source":         probe.CmdProbeInputs.Source,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PostChaos",
		})

		// waiting for initial delay
		if probe.RunProperties.InitialDelaySeconds != 0 {
			log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
			time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		}

		// triggering the cmd probe for the inline mode
		if probe.CmdProbeInputs.Source == "inline" {
			err = TriggerInlineCmdProbe(probe, resultDetails)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			// it will update the status of all the unrun probes as well
			if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PostChaos"); err != nil {
				return err
			}
		} else {

			execCommandDetails, err := CreateHelperPod(probe, resultDetails, clients, chaosDetails, probe.CmdProbeInputs.Source)
			if err != nil {
				return err
			}

			// triggering the cmd probe and storing the output into the out buffer
			err = TriggerSourceCmdProbe(probe, execCommandDetails, clients, resultDetails)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			// it will update the status of all the unrun probes as well
			if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PostChaos"); err != nil {
				return err
			}

			// get runId
			runID := GetRunIDFromProbe(resultDetails, probe.Name, probe.Type)

			// deleting the external pod which was created for cmd probe
			if err = DeleteProbePod(chaosDetails, clients, runID); err != nil {
				return err
			}
		}
	case "Continuous", "OnChaos":
		if probe.CmdProbeInputs.Source == "inline" {
			// it will check for the error, It will detect the error if any error encountered in probe during chaos
			err = CheckForErrorInContinuousProbe(resultDetails, probe.Name)
			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PostChaos"); err != nil {
				return err
			}
		} else {
			// it will check for the error, It will detect the error if any error encountered in probe during chaos
			err = CheckForErrorInContinuousProbe(resultDetails, probe.Name)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, "PostChaos"); err != nil {
				return err
			}
			// get runId
			runID := GetRunIDFromProbe(resultDetails, probe.Name, probe.Type)
			// deleting the external pod, which was created for cmd probe
			if err = DeleteProbePod(chaosDetails, clients, runID); err != nil {
				return err
			}

		}
	}
	return nil
}

//OnChaosCmdProbe trigger the cmd probe for DuringChaos phase
func OnChaosCmdProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch probe.Mode {
	case "OnChaos":

		//DISPLAY THE Cmd PROBE INFO
		log.InfoWithValues("[Probe]: The cmd probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Command":        probe.CmdProbeInputs.Command,
			"Comparator":     probe.CmdProbeInputs.Comparator,
			"Source":         probe.CmdProbeInputs.Source,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "DuringChaos",
		})
		if probe.CmdProbeInputs.Source == "inline" {
			go TriggerInlineOnChaosCmdProbe(probe, resultDetails, chaosDetails.ChaosDuration)
		} else {

			execCommandDetails, err := CreateHelperPod(probe, resultDetails, clients, chaosDetails, probe.CmdProbeInputs.Source)
			if err != nil {
				return err
			}

			// trigger the continuous cmd probe
			go TriggerSourceOnChaosCmdProbe(probe, execCommandDetails, clients, resultDetails, chaosDetails.ChaosDuration)
		}

	}
	return nil
}

// CreateHelperPod create the helper pod with the source image
// it will be created if the mode is not inline
func CreateHelperPod(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, sourceImage string) (litmusexec.PodDetails, error) {
	// Generate the run_id
	runID := GetRunID()
	SetRunIDForProbe(resultDetails, probe.Name, probe.Type, runID)

	// create the external pod with source image for cmd probe
	err := CreateProbePod(clients, chaosDetails, runID, sourceImage)
	if err != nil {
		return litmusexec.PodDetails{}, err
	}

	// verify the running status of external probe pod
	log.Info("[Status]: Checking the status of the probe pod")
	err = status.CheckApplicationStatus(chaosDetails.ChaosNamespace, "name="+chaosDetails.ExperimentName+"-probe-"+runID, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return litmusexec.PodDetails{}, errors.Errorf("probe pod is not in running state, err: %v", err)
	}

	// setting the attributes for the exec command
	execCommandDetails := litmusexec.PodDetails{}
	litmusexec.SetExecCommandAttributes(&execCommandDetails, chaosDetails.ExperimentName+"-probe-"+runID, chaosDetails.ExperimentName+"-probe", chaosDetails.ChaosNamespace)

	return execCommandDetails, nil
}
