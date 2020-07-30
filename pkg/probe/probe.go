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

var err error

// AddProbes contains the steps to trigger the probes
func AddProbes(chaosDetails *types.ChaosDetails, clients clients.ClientSets, resultDetails *types.ResultDetails) error {

	// get the probes from the chaosengine
	k8sProbes, httpProbes, cmdProbes, err := GetProbesFromEngine(chaosDetails, clients)
	if err != nil {
		return err
	}

	// it contains steps to prepare the k8s probe
	err = PrepareK8sProbe(k8sProbes, resultDetails)
	if err != nil {
		return err
	}

	// it contains steps to prepare cmd probe
	err = PrepareCmdProbe(cmdProbes, clients, chaosDetails, resultDetails)
	if err != nil {
		return err
	}

	// it contains steps to prepare http probe
	err = PrepareHTTPProbe(httpProbes, clients, chaosDetails, resultDetails)
	if err != nil {
		return err
	}

	return nil
}

// PrepareK8sProbe contains the steps to prepare the k8s probe
// k8s probe can be used to add the probe which needs client-go for command execution, no extra binaries/command
func PrepareK8sProbe(k8sProbes []v1alpha1.K8sProbeAttributes, resultDetails *types.ResultDetails) error {

	if k8sProbes != nil {

		for _, probe := range k8sProbes {

			//DISPLAY THE K8S PROBE INFO
			log.InfoWithValues("[Probe]: The k8s probe informations are as follows", logrus.Fields{
				"Name":            probe.Name,
				"Command":         probe.Inputs.Command,
				"Expected Result": probe.Inputs.ExpectedResult,
				"Run Properties":  probe.RunProperties,
			})

			// triggering the k8s probe and storing the output into the out buffer
			err = TriggerK8sProbe(probe, probe.Inputs.Command)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			if err != nil {
				SetProbeVerdict(resultDetails, "Better Luck Next Time", probe.Name, "K8sProbe")
				log.Infof("The %v k8s probe has been Failed", probe.Name)
				return err
			}
			resultDetails.ProbeCount++
			SetProbeVerdict(resultDetails, "Accepted", probe.Name, "K8sProbe")
			log.Infof("[Probe]: The %v probe has been Passed", probe.Name)
		}
	}
	return nil
}

//SetProbeVerdict ...
func SetProbeVerdict(resultDetails *types.ResultDetails, verdict, probeName, probeType string) {

	for index, probe := range resultDetails.ProbeDetails {
		if probe.Name == probeName && probe.Type == probeType {
			resultDetails.ProbeDetails[index].Verdict = verdict
			break
		}
	}
}

// TriggerK8sProbe is running the k8s probe command and storing the output into the out buffer
func TriggerK8sProbe(probe v1alpha1.K8sProbeAttributes, cmd string) error {

	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	err = retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			out, err := ParseCommandAndRun(cmd)
			if err != nil {
				return err
			}
			// Trim the extra whitespaces from the output and match the actual output with the expected output
			if strings.TrimSpace(string(out)) != probe.Inputs.ExpectedResult {
				return fmt.Errorf("The probe output didn't match with expected output, Probe Output: %v", string(out))
			}
			return nil
		})
	return err
}

// PrepareHTTPProbe contains the steps to prepare the http probe
// http probe can be used to add the probe which will curl an url and match the output
func PrepareHTTPProbe(httpProbes []v1alpha1.HTTPProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	if httpProbes != nil {

		for _, probe := range httpProbes {

			//DISPLAY THE K8S PROBE INFO
			log.InfoWithValues("[Probe]: The k8s probe informations are as follows", logrus.Fields{
				"Name":              probe.Name,
				"URL":               probe.Inputs.URL,
				"Expecected Result": probe.Inputs.ExpectedResult,
				"Run Properties":    probe.RunProperties,
			})

			// trigger the http probe and storing the output into the out buffer
			err = TriggerHTTPProbe(probe)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			if err != nil {
				SetProbeVerdict(resultDetails, "Better Luck Next Time", probe.Name, "HTTPProbe")
				log.Infof("The %v http probe has been Failed", probe.Name)
				return err
			}
			resultDetails.ProbeCount++
			SetProbeVerdict(resultDetails, "Accepted", probe.Name, "HTTPProbe")
			log.Infof("[Probe]: The %v probe has been Passed", probe.Name)
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
			out, err := ParseCommandAndRun(probe.Inputs.URL)
			if err != nil {
				return err
			}
			// Trim the extra whitespaces from the output and match the actual output with the expected output
			if strings.TrimSpace(string(out)) != probe.Inputs.ExpectedResult {
				return fmt.Errorf("The probe output didn't match with expected output, Probe Output: %v", string(out))
			}
			return nil
		})
	return err
}

// PrepareCmdProbe contains the steps to prepare the cmd probe
// cmd probe can be used to add the probe which will run the command which need a source(an external image) or
// any inline command which can be run without source image as well
func PrepareCmdProbe(cmdProbes []v1alpha1.CmdProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {
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
			// splitting the string to the list of strings/commands
			command := strings.Fields(probe.Inputs.Command)

			if probe.Inputs.Source == "inline" {

				// triggering the cmd probe and storing the output into the out buffer
				err = TriggerInlineCmdProbe(probe, command)

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

				// exec inside probe pod to get the details
				execCommandDetails := litmusexec.PodDetails{}
				litmusexec.SetExecCommandAttributes(&execCommandDetails, chaosDetails.ExperimentName+"-probe-"+runID, chaosDetails.ExperimentName+"-probe", chaosDetails.ChaosNamespace)

				// triggering the cmd probe and storing the output into the out buffer
				err = TriggerCmdProbe(probe, command, execCommandDetails, clients)

			}

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			if err != nil {
				SetProbeVerdict(resultDetails, "Better Luck Next Time", probe.Name, "CmdProbe")
				log.Infof("The %v cmd probe has been Failed", probe.Name)
				return err
			}
			resultDetails.ProbeCount++
			SetProbeVerdict(resultDetails, "Accepted", probe.Name, "CmdProbe")

			log.Infof("[Probe]: The %v probe has been Passed", probe.Name)

		}

	}
	return nil
}

// TriggerInlineCmdProbe trigger the cmd probe and storing the output into the out buffer
func TriggerInlineCmdProbe(probe v1alpha1.CmdProbeAttributes, command []string) error {

	// running the cmd probe command and storing the output into the out buffer
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	err = retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			var out, stderr bytes.Buffer
			cmd := exec.Command(command[0], command[1:]...)
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				log.Infof("Error String: %v", stderr.String())
				return fmt.Errorf("Unable to run the command, err: %v", err)
			}
			// Trim the extra whitespaces from the output and match the actual output with the expected output
			if strings.TrimSpace(out.String()) != probe.Inputs.ExpectedResult {
				log.Infof("The %v cmd probe has been Failed", probe.Name)
				return fmt.Errorf("The probe output didn't match with expected output, %v", out.String())
			}
			return nil
		})
	return err
}

// TriggerCmdProbe trigger the cmd probe inside the external pod and storing the output into the out buffer
func TriggerCmdProbe(probe v1alpha1.CmdProbeAttributes, command []string, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets) error {

	// running the cmd probe command and matching the ouput
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	err = retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
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

// GetProbesFromEngine fetch teh details of the probes from the chaosengines
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

// SetProbesInChaosResult set the probe inside chaos result
func SetProbesInChaosResult(chaosDetails *types.ChaosDetails, clients clients.ClientSets, chaosresult *types.ResultDetails) error {

	probeDetails := []types.ProbeDetails{}
	probeDetail := types.ProbeDetails{}
	// get the probes from the chaosengine
	k8sProbes, httpProbes, cmdProbes, err := GetProbesFromEngine(chaosDetails, clients)
	if err != nil {
		return err
	}

	for _, probe := range k8sProbes {
		probeDetail.Name = probe.Name
		probeDetail.Type = "K8sProbe"
		probeDetail.Verdict = "Awaited"
		probeDetails = append(probeDetails, probeDetail)
	}

	for _, probe := range httpProbes {
		probeDetail.Name = probe.Name
		probeDetail.Type = "HTTPProbe"
		probeDetail.Verdict = "Awaited"
		probeDetails = append(probeDetails, probeDetail)
	}

	for _, probe := range cmdProbes {
		probeDetail.Name = probe.Name
		probeDetail.Type = "CmdProbe"
		probeDetail.Verdict = "Awaited"
		probeDetails = append(probeDetails, probeDetail)
	}

	chaosresult.ProbeDetails = probeDetails

	return nil
}

// ParseCommandAndRun ..
func ParseCommandAndRun(command string) ([]byte, error) {
	// split the pipe commands
	commands := []*exec.Cmd{}
	cmd := strings.Split(command, "|")
	for index := range cmd {
		// trim extra space from the commands
		cmd[index] = strings.TrimSpace(cmd[index])
		values := strings.Fields(cmd[index])
		command := exec.Command(values[0], values[1:]...)
		commands = append(commands, command)
	}

	out, err := PipeCommand(commands...)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// PipeCommand ...
func PipeCommand(commands ...*exec.Cmd) ([]byte, error) {
	var out, stderr bytes.Buffer
	for i, command := range commands[:len(commands)-1] {
		command.Stdout = &out
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			log.Infof("Error String: %v", stderr.String())
			return nil, fmt.Errorf("Unable to run the command, err: %v", err)
		}
		fmt.Printf("output1: %v", out.String())
		commands[i+1].Stdin = &out
		commands[i+1].StdinPipe()

	}

	commands[len(commands)-1].Stdout = &out
	commands[len(commands)-1].Stderr = &stderr
	if err := commands[len(commands)-1].Run(); err != nil {
		log.Infof("Error String: %v", stderr.String())
		return nil, fmt.Errorf("Unable to run the command, err: %v", err)
	}
	fmt.Printf("output2: %v", out.String())
	return out.Bytes(), nil
}
