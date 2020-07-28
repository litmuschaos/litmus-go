package probe

import (
	"bytes"
	"fmt"
	"math/rand"
	"os/exec"
	"strings"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AddProbes ...
func AddProbes(chaosDetails *types.ChaosDetails, clients clients.ClientSets) error {

	// get the probes from the chaosengine
	k8sProbes, httpProbes, cmdProbes, err := GetProbesFromEngine(chaosDetails, clients)
	if err != nil {
		return err
	}

	err = CheckForK8sProbe(k8sProbes)
	if err != nil {
		return err
	}

	err = CheckForCmdProbe(cmdProbes, clients, chaosDetails)
	if err != nil {
		return err
	}

	err = CheckForHTTPProbe(httpProbes)
	if err != nil {
		return err
	}

	return nil

}

// CheckForK8sProbe ...
func CheckForK8sProbe(k8sProbes []v1alpha1.K8sProbeAttributes) error {

	if k8sProbes != nil {

		for _, probe := range k8sProbes {

			//DISPLAY THE K8S PROBE INFO
			log.InfoWithValues("[Probe]: The k8s probe informations are as follows", logrus.Fields{
				"Name":              probe.Name,
				"Command":           probe.Inputs.Command,
				"Expecected Result": probe.Inputs.ExpectedResult,
			})

			command := strings.Fields(probe.Inputs.Command)
			cmd := exec.Command(command[0], command[1:]...)
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				log.Infof("Error String: %v", stderr.String())
				return fmt.Errorf("Unable to run the command, err: %v", err)
			}

			// Trim the extra whitespaces from the output and match the actual output with the expected output
			if strings.TrimSpace(out.String()) != probe.Inputs.ExpectedResult {
				log.Infof("The %v k8s probe has been Failed", probe.Name)
				return fmt.Errorf("The probe output didn't match with expected output, %v", out.String())
			}

			log.Infof("[Probe]: The %v probe has been Passed", probe.Name)

		}

	}
	return nil
}

// CheckForHTTPProbe ...
func CheckForHTTPProbe(httpProbes []v1alpha1.HTTPProbeAttributes) error {

	if httpProbes != nil {

		for _, probe := range httpProbes {

			//DISPLAY THE K8S PROBE INFO
			log.InfoWithValues("[Probe]: The k8s probe informations are as follows", logrus.Fields{
				"Name":              probe.Name,
				"URL":               probe.Inputs.URL,
				"Expecected Result": probe.Inputs.ExpectedResult,
			})

			cmd := exec.Command("curl", probe.Inputs.URL)
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				log.Infof("Error String: %v", stderr.String())
				return fmt.Errorf("Unable to run the command, err: %v", err)
			}

			// Trim the extra whitespaces from the output and match the actual output with the expected output
			if strings.TrimSpace(out.String()) != probe.Inputs.ExpectedResult {
				log.Infof("The %v http probe has been Failed", probe.Name)
				return fmt.Errorf("The probe output didn't match with expected output, %v", out.String())
			}

			log.Infof("[Probe]: The %v probe has been Passed", probe.Name)

		}

	}
	return nil
}

// CheckForCmdProbe ...
func CheckForCmdProbe(cmdProbes []v1alpha1.CmdProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {
	var actualOutput string
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
			})
			command := strings.Fields(probe.Inputs.Command)

			if probe.Inputs.Source == "inline" {
				var out, stderr bytes.Buffer
				cmd := exec.Command(command[0], command[1:]...)
				cmd.Stdout = &out
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					log.Infof("Error String: %v", stderr.String())
					return fmt.Errorf("Unable to run the command, err: %v", err)
				}
				actualOutput = out.String()

			} else {

				// create pod w/ image
				err := CreateCmdProbe(clients, chaosDetails, runID, probe.Inputs.Source)
				if err != nil {
					return err
				}

				// status check
				log.Info("[Status]: Checking the status of the probe pod")
				err = status.CheckApplicationStatus(chaosDetails.ChaosNamespace, "name="+chaosDetails.ExperimentName+"-probe-"+runID, clients)
				if err != nil {
					return errors.Errorf("probe pod is not in running state, err: %v", err)
				}

				// exec
				execCommandDetails := litmusexec.PodDetails{}
				litmusexec.SetExecCommandAttributes(&execCommandDetails, chaosDetails.ExperimentName+"-probe-"+runID, chaosDetails.ExperimentName+"-probe", chaosDetails.ChaosNamespace)
				actualOutput, err = litmusexec.Exec(&execCommandDetails, clients, command)
				if err != nil {
					return errors.Errorf("Unable to get output of cmd command due to err: %v", err)
				}

			}

			// Trim the extra whitespaces from the output and match the actual output with the expected output
			if strings.TrimSpace(actualOutput) != probe.Inputs.ExpectedResult {
				log.Infof("The %v cmd probe has been Failed", probe.Name)
				return fmt.Errorf("The probe output didn't match with expected output, %v", actualOutput)
			}

			log.Infof("[Probe]: The %v probe has been Passed", probe.Name)

		}

	}
	return nil
}

// GetProbesFromEngine ...
func GetProbesFromEngine(chaosDetails *types.ChaosDetails, clients clients.ClientSets) ([]v1alpha1.K8sProbeAttributes, []v1alpha1.HTTPProbeAttributes, []v1alpha1.CmdProbeAttributes, error) {

	engine, err := clients.LitmusClient.ChaosEngines(chaosDetails.ChaosNamespace).Get(chaosDetails.EngineName, v1.GetOptions{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Unable to Get the chaosengine due to %v", err)
	}

	// define all the probes
	var k8sProbes []v1alpha1.K8sProbeAttributes
	var httpProbes []v1alpha1.HTTPProbeAttributes
	var cmdProbes []v1alpha1.CmdProbeAttributes

	// get all the probes define inside chaosengine for corresponding experiment
	experimentSpec := engine.Spec.Experiments
	for _, experiment := range experimentSpec {

		if experiment.Name == chaosDetails.ExperimentName {

			k8sProbes = experiment.Spec.K8sProbe
			httpProbes = experiment.Spec.HTTPProbe
			cmdProbes = experiment.Spec.CmdProbe
		}
	}

	return k8sProbes, httpProbes, cmdProbes, nil
}

// CreateCmdProbe ...
func CreateCmdProbe(clients clients.ClientSets, chaosDetails *types.ChaosDetails, runID, source string) error {

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
						"bin/sh",
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

// GetRunID generate a random string
func GetRunID() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")
	runID := make([]rune, 6)
	for i := range runID {
		runID[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(runID)
}
