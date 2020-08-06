package probe

import (
	"bytes"
	"fmt"
	"math/rand"
	"os/exec"
	"strings"
	"time"

	"github.com/kyokomi/emoji"
	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
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
// cmd probe can be used to add the probe which will run the command which need a source(an external image) or
// any inline command which can be run without source image as well
func PrepareCmdProbe(cmdProbes []v1alpha1.CmdProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {
	// Generate the run_id
	runID := GetRunID()

	if cmdProbes != nil {
		for _, probe := range cmdProbes {

			//DISPLAY THE K8S PROBE INFO
			log.InfoWithValues("[Probe]: The cmd probe informations are as follows", logrus.Fields{
				"Name":            probe.Name,
				"Command":         probe.Inputs.Command,
				"Expected Result": probe.Inputs.ExpectedResult,
				"Source":          probe.Inputs.Source,
				"Run Properties":  probe.RunProperties,
				"Mode":            probe.Mode,
			})

			if probe.Inputs.Source == "inline" {

				//division on the basis of mode
				// trigger probes for the edge modes
				if (probe.Mode == "SOT" && phase == "PreChaos") || (probe.Mode == "EOT" && phase == "PostChaos") || probe.Mode == "Edge" {

					// triggering the cmd probe for the inline mode
					err = TriggerInlineCmdProbe(probe)
					// failing the probe, if the success condition doesn't met after the retry & timeout combinations
					MarkedVerdictInEnd(err, probe, resultDetails, phase)
					if err != nil {
						return err
					}
				}
				// trigger probes for the continuous mode
				if probe.Mode == "Continuous" && phase == "PreChaos" {
					go TriggerInlineContinuousCmdProbe(probe, resultDetails)
				}
				// verify the continuous mode
				if probe.Mode == "Continuous" && phase == "PostChaos" {
					err = ContinuousTrial(resultDetails, cmdProbes)
					// failing the probe, if the success condition doesn't met after the retry & timeout combinations
					MarkedVerdictInEnd(err, probe, resultDetails, phase)
					if err != nil {
						return err
					}
				}

			} else {

				// create the external pod with source image for cmd probe
				err := CreateProbePod(clients, chaosDetails, runID, probe.Inputs.Source)
				if err != nil {
					return err
				}

				// verify the running status of external probe pod
				log.Info("[Status]: Checking the status of the probe pod")
				err = status.CheckApplicationStatus(chaosDetails.ChaosNamespace, "name="+chaosDetails.ExperimentName+"-probe-"+runID, chaosDetails.Timeout, chaosDetails.Delay, clients)
				if err != nil {
					return errors.Errorf("probe pod is not in running state, err: %v", err)
				}

				// setting the attributes for the exec command
				execCommandDetails := litmusexec.PodDetails{}
				litmusexec.SetExecCommandAttributes(&execCommandDetails, chaosDetails.ExperimentName+"-probe-"+runID, chaosDetails.ExperimentName+"-probe", chaosDetails.ChaosNamespace)

				//division on the basis of mode
				// trigger probes for the edge modes
				if (probe.Mode == "SOT" && phase == "PreChaos") || (probe.Mode == "EOT" && phase == "PostChaos") || probe.Mode == "Edge" {

					// triggering the cmd probe and storing the output into the out buffer
					err = TriggerCmdProbe(probe, execCommandDetails, clients)
					// failing the probe, if the success condition doesn't met after the retry & timeout combinations

					// failing the probe, if the success condition doesn't met after the retry & timeout combinations
					MarkedVerdictInEnd(err, probe, resultDetails, phase)
					if err != nil {
						return err
					}

					// deleting the external pod which was created for cmd probe
					if err = DeleteProbePod(chaosDetails, clients, runID); err != nil {
						return err
					}
				}
				// trigger probes for the continuous mode
				if probe.Mode == "Continuous" && phase == "PreChaos" {
					go TriggerContinuousCmdProbe(probe, execCommandDetails, clients, resultDetails)
				}
				// verify the continuous mode
				if probe.Mode == "Continuous" && phase == "PostChaos" {
					err = ContinuousTrial(resultDetails, cmdProbes)

					// failing the probe, if the success condition doesn't met after the retry & timeout combinations
					MarkedVerdictInEnd(err, probe, resultDetails, phase)
					if err != nil {
						return err
					}

					// deleting the external pod which was created for cmd probe
					if err = DeleteProbePod(chaosDetails, clients, runID); err != nil {
						return err
					}
				}

			}
		}

	}
	return nil
}

// TriggerInlineCmdProbe trigger the cmd probe and storing the output into the out buffer
func TriggerInlineCmdProbe(probe v1alpha1.CmdProbeAttributes) error {

	// running the cmd probe command and storing the output into the out buffer
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	err = retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			// parse the command for the pipe, if command contains awk, grep commands
			out, err := ParseCommandAndRun(probe.Inputs.Command)
			if err != nil {
				return err
			}
			// Trim the extra whitespaces from the output and match the actual output with the expected output
			if strings.TrimSpace(string(out)) != probe.Inputs.ExpectedResult {
				log.Infof("[Retry]The %v cmd probe has been Failed", probe.Name)
				return fmt.Errorf("The probe output didn't match with expected output, %v", string(out))
			}
			return nil
		})
	return err
}

// TriggerInlineContinuousCmdProbe trigger the inline continuous cmd probes
func TriggerInlineContinuousCmdProbe(probe v1alpha1.CmdProbeAttributes, chaosresult *types.ResultDetails) {
	// it trigger the inline cmd probe for the entire duration of chaos and it fails when get an not nil error
	// it mark the not nill error for the probes, if any
	for {
		err = TriggerInlineCmdProbe(probe)
		if err != nil {
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].C1 = err
					break
				}

			}
			break
		}

	}

}

// TriggerCmdProbe trigger the cmd probe inside the external pod and storing the output into the out buffer
func TriggerCmdProbe(probe v1alpha1.CmdProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets) error {

	// running the cmd probe command and matching the ouput
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	err = retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			command := append([]string{"/bin/sh", "-c"}, probe.Inputs.Command)
			// exec inside the external pod to get the o/p of given command
			output, err := litmusexec.Exec(&execCommandDetails, clients, command)
			if err != nil {
				return errors.Errorf("Unable to get output of cmd command due to err: %v", err)
			}
			// Trim the extra whitespaces from the output and match the actual output with the expected output
			if strings.TrimSpace(output) != probe.Inputs.ExpectedResult {
				log.Infof("The %v cmd probe has been Failed", probe.Name)
				return fmt.Errorf("The probe output didn't match with expected output, %v", output)
			}
			return nil
		})
	return err
}

// TriggerContinuousCmdProbe trigger the continuous cmd probes
func TriggerContinuousCmdProbe(probe v1alpha1.CmdProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, chaosresult *types.ResultDetails) {
	// it trigger the cmd probe for the entire duration of chaos and it fails when get an not nil error
	// it mark the not nill error for the probes, if any
	for {
		err = TriggerCmdProbe(probe, execCommandDetails, clients)
		if err != nil {
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].C1 = err
					break
				}

			}
			break
		}

	}

}

// CreateProbePod creates an extrenal pod with source image for the cmd probe
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
					ImagePullPolicy: apiv1.PullAlways,
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

	err := clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).Delete(chaosDetails.ExperimentName+"-probe-"+runID, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	// waiting for the termination of the pod
	err = retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: chaosDetails.ExperimentName + "-probe-" + runID})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Probe Pod is not deleted yet, err: %v", err)
			}
			return nil
		})

	return err
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

// ParseCommandAndRun parse the command for pipe(|) operator
// it used when command contains awk, grep commands
func ParseCommandAndRun(command string) ([]byte, error) {
	commands := []*exec.Cmd{}
	// split the command by pipe operator
	cmd := strings.Split(command, "|")
	// trim extra space from the commands and convert them to exec.Cmd form
	for index := range cmd {

		cmd[index] = strings.TrimSpace(cmd[index])
		values := strings.Fields(cmd[index])
		command := exec.Command(values[0], values[1:]...)
		commands = append(commands, command)
	}

	// running the commands and joined them by pipe for multiple commands
	// otherwise run the command normally
	out, err := PipeCommand(commands...)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// PipeCommand joined the command by pipe and run them
// and return the final output
func PipeCommand(commands ...*exec.Cmd) ([]byte, error) {
	len := len(commands)
	// running the commads [1,n-1]
	for i, command := range commands[:len-1] {
		var out, stderr bytes.Buffer
		command.Stdout = &out
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			return nil, fmt.Errorf("Unable to run the command, err: %v", err)
		}
		// use the ouput of (i-1)th output as input of ith command
		commands[i+1].Stdin = &out
		// adding the pipe to the input of command
		commands[i+1].StdinPipe()

	}

	// running the nth command and provide o/p of (n-1)th command's ouput as input
	var out, stderr bytes.Buffer
	commands[len-1].Stdout = &out
	commands[len-1].Stderr = &stderr
	if err := commands[len-1].Run(); err != nil {
		return nil, fmt.Errorf("Unable to run the command, err: %v", err)
	}
	return out.Bytes(), nil
}

//ContinuousTrial verify the probe result for the continuous cmd probe
func ContinuousTrial(resultDetails *types.ResultDetails, cmdProbes []v1alpha1.CmdProbeAttributes) error {

	for _, probe := range cmdProbes {
		if probe.Mode == "" {
			for index, probe1 := range resultDetails.ProbeDetails {
				if probe1.Name == probe.Name {
					err = resultDetails.ProbeDetails[index].C1
					return err
				}
			}
		}
	}

	return nil
}

// MarkedVerdictInEnd ...
func MarkedVerdictInEnd(err error, probe v1alpha1.CmdProbeAttributes, resultDetails *types.ResultDetails, phase string) {
	// failing the probe, if the success condition doesn't met after the retry & timeout combinations
	if err != nil {
		log.Infof("[Probe]: %v probe has been Failed %v", probe.Name, emoji.Sprint(":cry:"))
		SetProbeVerdictAfterFailure(resultDetails)
	}
	resultDetails.ProbeCount++
	SetProbeVerdict(resultDetails, "Passed", probe.Name, "CmdProbe", probe.Mode, phase)
	log.Infof("[Probe]: %v probe has been Passed %v", probe.Name, emoji.Sprint(":smile:"))
	resultDetails.PassedProbe = append(resultDetails.PassedProbe, probe.Name+"-"+phase)
}
