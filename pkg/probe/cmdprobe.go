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
func PrepareCmdProbe(cmdProbes []v1alpha1.CmdProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string, eventsDetails *types.EventDetails) error {

	if cmdProbes != nil {
		for _, probe := range cmdProbes {

			//DISPLAY THE cmd PROBE INFO
			if !((probe.Mode == "SOT" || probe.Mode == "Continuous") && phase == "PreChaos") {
				log.InfoWithValues("[Probe]: The cmd probe information is as follows", logrus.Fields{
					"Name":            probe.Name,
					"Command":         probe.Inputs.Command,
					"Expected Result": probe.Inputs.ExpectedResult,
					"Source":          probe.Inputs.Source,
					"Run Properties":  probe.RunProperties,
					"Mode":            probe.Mode,
				})
			}

			// triggering the cmd probe for the inline mode
			if probe.Inputs.Source == "inline" {

				// triggering probes on the basis of mode & phase so that probe will only run when they are requested to run
				// if mode is SOT & phase is PreChaos, it will trigger Probes in PreChaos section
				// if mode is EOT & phase is PostChaos, it will trigger Probes in PostChaos section
				// if mode is Edge then independent of phase, it will trigger Probes in both Pre/Post Chaos section
				if (probe.Mode == "SOT" && phase == "PreChaos") || (probe.Mode == "EOT" && phase == "PostChaos") || probe.Mode == "Edge" {

					err = TriggerInlineCmdProbe(probe)

					// failing the probe, if the success condition doesn't met after the retry & timeout combinations
					// it will update the status of all the unrun probes as well
					if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, "CmdProbe", phase); err != nil {
						return err
					}
				}
				// trigger probes for the continuous mode
				if probe.Mode == "Continuous" && phase == "PreChaos" {
					go TriggerInlineContinuousCmdProbe(probe, resultDetails)
				}
				// verify the continuous probe status
				// marked the result of continuous probe
				if probe.Mode == "Continuous" && phase == "PostChaos" {
					// it will check for the error, It will detect the error if any error encountered in probe during chaos
					err = CheckForErrorInContinuousProbe(resultDetails, probe.Name)
					// failing the probe, if the success condition doesn't met after the retry & timeout combinations
					if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, "CmdProbe", phase); err != nil {
						return err
					}
				}
			} else {

				if (probe.Mode == "SOT" && phase == "PreChaos") || (probe.Mode == "EOT" && phase == "PostChaos") || probe.Mode == "Edge" {
					// Generate the run_id
					runID := GetRunID()

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

					// triggering the cmd probe and storing the output into the out buffer
					err = TriggerCmdProbe(probe, execCommandDetails, clients)

					// failing the probe, if the success condition doesn't met after the retry & timeout combinations
					// it will update the status of all the unrun probes as well
					if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, "CmdProbe", phase); err != nil {
						return err
					}

					// deleting the external pod which was created for cmd probe
					if err = DeleteProbePod(chaosDetails, clients, runID); err != nil {
						return err
					}
				}
				// trigger probes for the continuous mode
				if probe.Mode == "Continuous" && phase == "PreChaos" {
					// Generate the run_id
					runID := GetRunID()
					SetRunIDForProbe(resultDetails, probe.Name, "CmdProbe", runID)

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

					// trigger the continuous cmd probe
					go TriggerContinuousCmdProbe(probe, execCommandDetails, clients, resultDetails)
				}
				// verify the continuous mode
				// marked the result of continuous probe
				if probe.Mode == "Continuous" && phase == "PostChaos" {
					// it will check for the error, It will detect the error if any error encountered in probe during chaos
					err = CheckForErrorInContinuousProbe(resultDetails, probe.Name)

					// failing the probe, if the success condition doesn't met after the retry & timeout combinations
					if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, "CmdProbe", phase); err != nil {
						return err
					}
					// get runId
					runID := GetRunIDFromProbe(resultDetails, probe.Name, "CmdProbe")
					// deleting the external pod, which was created for cmd probe
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
			var out bytes.Buffer
			// run the inline command probe
			cmd := exec.Command("/bin/sh", "-c", probe.Inputs.Command)
			cmd.Stdout = &out
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("Unable to run command, err: %v", err)
			}
			// Trim the extra whitespaces from the output and match the actual output with the expected output
			if strings.TrimSpace(out.String()) != probe.Inputs.ExpectedResult {
				log.Warnf("The %v cmd probe has been Failed", probe.Name)
				return fmt.Errorf("The probe output didn't match with expected output, %v", out.String())
			}
			return nil
		})
	return err
}

// TriggerCmdProbe trigger the cmd probe inside the external pod
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
				log.Warnf("The %v cmd probe has been Failed", probe.Name)
				return fmt.Errorf("The probe output didn't match with expected output, %v", output)
			}
			return nil
		})
	return err
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
			RestartPolicy:      apiv1.RestartPolicyNever,
			ServiceAccountName: "litmus",
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

	// waiting till the termination of the pod
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

// TriggerInlineContinuousCmdProbe trigger the inline continuous cmd probes
func TriggerInlineContinuousCmdProbe(probe v1alpha1.CmdProbeAttributes, chaosresult *types.ResultDetails) {
	// it trigger the inline cmd probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
	for {
		err = TriggerInlineCmdProbe(probe)
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

	}

}

// TriggerContinuousCmdProbe trigger the continuous cmd probes having need some external source image
func TriggerContinuousCmdProbe(probe v1alpha1.CmdProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, chaosresult *types.ResultDetails) {
	// it trigger the cmd probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
	for {
		err = TriggerCmdProbe(probe, execCommandDetails, clients)
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

	}

}
