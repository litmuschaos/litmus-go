package probe

import (
	"bytes"
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
	"os/exec"
	"reflect"
	"strings"
	"time"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	cmp "github.com/litmuschaos/litmus-go/pkg/probe/comparator"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// prepareCmdProbe contains the steps to prepare the cmd probe
// cmd probe can be used to add the command probes
// it can be of two types one: which need a source(an external image)
// another: any inline command which can be run without source image, directly via go-runner image
func prepareCmdProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, phase string) error {

	switch strings.ToLower(phase) {
	case "prechaos":
		if err := preChaosCmdProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "postchaos":
		if err := postChaosCmdProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "duringchaos":
		if err := onChaosCmdProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeCmdProbe, Target: fmt.Sprintf("{name: %v}", probe.Name), Reason: fmt.Sprintf("phase '%s' not supported in the cmd probe", phase)}
	}
	return nil
}

// triggerInlineCmdProbe trigger the cmd probe and storing the output into the out buffer
func triggerInlineCmdProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails) error {
	var description string

	// It parses the templated command and return normal string
	// if command doesn't have template, it will return the same command
	probe.CmdProbeInputs.Command, err = parseCommand(probe.CmdProbeInputs.Command, resultDetails)
	if err != nil {
		return err
	}

	// running the cmd probe command and storing the output into the out buffer
	// it will retry for some retry count, in each iteration of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the interval and again execute the command until timeout expires
	if err := retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			var out, errOut bytes.Buffer
			// run the inline command probe
			cmd := exec.Command("/bin/sh", "-c", probe.CmdProbeInputs.Command)
			cmd.Stdout = &out
			cmd.Stderr = &errOut
			if err := cmd.Run(); err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeCmdProbe, Target: fmt.Sprintf("{name: %v}", probe.Name), Reason: fmt.Sprintf("unable to run command: %s", errOut.String())}
			}

			rc := getAndIncrementRunCount(resultDetails, probe.Name)
			description, err = validateResult(probe.CmdProbeInputs.Comparator, probe.Name, strings.TrimSpace(out.String()), rc)
			if err != nil {
				log.Errorf("the %v cmd probe has been Failed, err: %v", probe.Name, err)
				return err
			}

			probes := types.ProbeArtifact{}
			probes.ProbeArtifacts.Register = strings.TrimSpace(out.String())
			resultDetails.ProbeArtifacts[probe.Name] = probes
			return nil
		}); err != nil {
		return err
	}

	setProbeDescription(resultDetails, probe, description)
	return nil
}

// triggerSourceCmdProbe trigger the cmd probe inside the external pod
func triggerSourceCmdProbe(probe v1alpha1.ProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, resultDetails *types.ResultDetails) error {
	var description string

	// It parse the templated command and return normal string
	// if command doesn't have template, it will return the same command
	probe.CmdProbeInputs.Command, err = parseCommand(probe.CmdProbeInputs.Command, resultDetails)
	if err != nil {
		return err
	}

	// running the cmd probe command and matching the output
	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	if err := retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			command := append([]string{"/bin/sh", "-c"}, probe.CmdProbeInputs.Command)
			// exec inside the external pod to get the o/p of given command
			output, err := litmusexec.Exec(&execCommandDetails, clients, command)
			if err != nil {
				return stacktrace.Propagate(err, "unable to get output of cmd command")
			}

			rc := getAndIncrementRunCount(resultDetails, probe.Name)
			if description, err = validateResult(probe.CmdProbeInputs.Comparator, probe.Name, strings.TrimSpace(output), rc); err != nil {
				log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
				return err
			}

			probes := types.ProbeArtifact{}
			probes.ProbeArtifacts.Register = strings.TrimSpace(output)
			resultDetails.ProbeArtifacts[probe.Name] = probes
			return nil
		}); err != nil {
		return err
	}

	setProbeDescription(resultDetails, probe, description)
	return nil
}

// createProbePod creates an external pod with source image for the cmd probe
func createProbePod(clients clients.ClientSets, chaosDetails *types.ChaosDetails, runID string, source v1alpha1.SourceDetails, probeName string) error {
	//deriving serviceAccount name for the probe pod
	svcAccount, err := getServiceAccount(chaosDetails.ChaosNamespace, chaosDetails.ChaosPodName, probeName, clients)
	if err != nil {
		return stacktrace.Propagate(err, "unable to get the serviceAccountName")
	}

	expEnv, volume, expVolumeMount := inheritInputs(clients, chaosDetails.ChaosNamespace, chaosDetails.ChaosPodName, source)

	cmdProbe := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        chaosDetails.ExperimentName + "-probe-" + runID,
			Namespace:   chaosDetails.ChaosNamespace,
			Labels:      getProbeLabels(source.Labels, chaosDetails, runID),
			Annotations: source.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy:      apiv1.RestartPolicyNever,
			HostNetwork:        source.HostNetwork,
			ServiceAccountName: svcAccount,
			Volumes:            volume,
			NodeSelector:       source.NodeSelector,
			ImagePullSecrets:   source.ImagePullSecrets,
			Containers: []apiv1.Container{
				{
					Name:            chaosDetails.ExperimentName + "-probe",
					Image:           source.Image,
					ImagePullPolicy: apiv1.PullPolicy(getProbeImagePullPolicy(string(source.ImagePullPolicy), chaosDetails)),
					Command:         getProbeCmd(source.Command),
					Args:            getProbeArgs(source.Args),
					Resources:       chaosDetails.Resources,
					Env:             expEnv,
					SecurityContext: &apiv1.SecurityContext{
						Privileged: &source.Privileged,
					},
					VolumeMounts: expVolumeMount,
				},
			},
		},
	}

	_, err = clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).Create(context.Background(), cmdProbe, v1.CreateOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeCmdProbe, Target: fmt.Sprintf("{name: %v}", probeName), Reason: err.Error()}
	}

	return nil
}

// inheritInputs will inherit the experiment details(ENVs and volumes) to the probe pod based on inheritInputs flag
func inheritInputs(clients clients.ClientSets, chaosNS, chaosPodName string, source v1alpha1.SourceDetails) ([]apiv1.EnvVar, []apiv1.Volume, []apiv1.VolumeMount) {

	if !source.InheritInputs {
		return source.ENVList, source.Volumes, source.VolumesMount
	}
	expEnv, expVolumeMount, volume := getEnvAndVolumeMountFromExperiment(clients, chaosNS, chaosPodName)

	expEnv = append(expEnv, source.ENVList...)
	expVolumeMount = append(expVolumeMount, source.VolumesMount...)
	volume = append(volume, source.Volumes...)

	return expEnv, volume, expVolumeMount

}

// getEnvAndVolumeMountFromExperiment get the env and volumeMount from the experiment pod and add in the probe pod
func getEnvAndVolumeMountFromExperiment(clients clients.ClientSets, chaosNamespace, podName string) ([]apiv1.EnvVar, []apiv1.VolumeMount, []apiv1.Volume) {

	envVarList := make([]apiv1.EnvVar, 0)
	volumeMountList := make([]apiv1.VolumeMount, 0)
	volumeList := make([]apiv1.Volume, 0)

	expPod, err := clients.KubeClient.CoreV1().Pods(chaosNamespace).Get(context.Background(), podName, v1.GetOptions{})
	if err != nil {
		log.Errorf("Unable to get the experiment pod, err: %v", err)
		return nil, nil, nil
	}

	expContainerName := expPod.Labels["job-name"]

	for _, container := range expPod.Spec.Containers {
		if container.Name == expContainerName {
			envVarList = append(envVarList, container.Env...)
		}
		for _, volumeMount := range container.VolumeMounts {
			// NOTE: one can add custom volume mount from probe spec
			if volumeMount.Name == "cloud-secret" {
				volumeMountList = append(volumeMountList, volumeMount)
			}
		}
	}

	for _, vol := range expPod.Spec.Volumes {
		// NOTE: one can add custom volume from probe spec
		if vol.Name == "cloud-secret" {
			volumeList = append(volumeList, vol)
		}
	}

	return envVarList, volumeMountList, volumeList
}

// getProbeLabels adding provided labels to probePod
func getProbeLabels(sourceLabels map[string]string, chaosDetails *types.ChaosDetails, runID string) map[string]string {

	envDetails := map[string]string{
		"name":     chaosDetails.ExperimentName + "-probe-" + runID,
		"chaosUID": string(chaosDetails.ChaosUID),
	}

	for key, element := range sourceLabels {
		envDetails[key] = element
	}
	return envDetails
}

// getProbeArgs adding provided args to probePod
func getProbeArgs(sourceArgs []string) []string {
	if len(sourceArgs) == 0 {
		return []string{"-c", "sleep 10000"}
	}
	return sourceArgs
}

// getProbeImagePullPolicy adds the image pull policy to the source pod
func getProbeImagePullPolicy(policy string, chaosDetails *types.ChaosDetails) string {
	if policy == "" {
		return chaosDetails.ProbeImagePullPolicy
	}
	return policy
}

// getProbeCmd adding provided command to probePod
func getProbeCmd(sourceCmd []string) []string {
	if len(sourceCmd) == 0 {
		return []string{"/bin/sh"}
	}
	return sourceCmd
}

// deleteProbePod deletes the probe pod and wait until it got terminated
func deleteProbePod(chaosDetails *types.ChaosDetails, clients clients.ClientSets, runID, probeName string) error {

	if err := clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).Delete(context.Background(), chaosDetails.ExperimentName+"-probe-"+runID, v1.DeleteOptions{}); err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeCmdProbe, Target: fmt.Sprintf("{name: %v}", probeName), Reason: err.Error()}
	}

	// waiting till the termination of the pod
	return retry.
		Times(uint(chaosDetails.Timeout / chaosDetails.Delay)).
		Wait(time.Duration(chaosDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(chaosDetails.ChaosNamespace).List(context.Background(), v1.ListOptions{LabelSelector: chaosDetails.ExperimentName + "-probe-" + runID})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeCmdProbe, Target: fmt.Sprintf("{name: %v}", probeName), Reason: fmt.Sprintf("failed to list probe pod: %s", err.Error())}
			} else if len(podSpec.Items) != 0 {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeCmdProbe, Target: fmt.Sprintf("{name: %v}", probeName), Reason: "probe pod is not deleted within timeout"}
			}
			return nil
		})
}

// triggerInlineContinuousCmdProbe trigger the inline continuous cmd probes
func triggerInlineContinuousCmdProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) {
	var isExperimentFailed bool
	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
	}

	// it trigger the inline cmd probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
loop:
	for {
		err := triggerInlineCmdProbe(probe, chaosresult)
		// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
		if err != nil {
			err = addProbePhase(err, string(chaosDetails.Phase))
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
					chaosresult.ProbeDetails[index].Status.Description = getDescription(err)
					log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
					isExperimentFailed = true
					break loop
				}
			}
		}
		// waiting for the probe polling interval
		time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
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

// triggerInlineOnChaosCmdProbe trigger the inline onchaos cmd probes
func triggerInlineOnChaosCmdProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) {
	var isExperimentFailed bool
	duration := chaosDetails.ChaosDuration
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
			// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
			if err = triggerInlineCmdProbe(probe, chaosresult); err != nil {
				err = addProbePhase(err, string(chaosDetails.Phase))
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
						chaosresult.ProbeDetails[index].Status.Description = getDescription(err)
						log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
						isExperimentFailed = true
						break loop
					}
				}
			}
			// waiting for the probe polling interval
			time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
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

// triggerSourceOnChaosCmdProbe trigger the onchaos cmd probes having need some external source image
func triggerSourceOnChaosCmdProbe(probe v1alpha1.ProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) {

	var isExperimentFailed bool
	duration := chaosDetails.ChaosDuration
	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		duration = math.Maximum(0, duration-probe.RunProperties.InitialDelaySeconds)
	}

	endTime := time.After(time.Duration(duration) * time.Second)

	// it trigger the cmd probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
loop:
	for {
		select {
		case <-endTime:
			log.Infof("[Chaos]: Time is up for the %v probe", probe.Name)
			endTime = nil
			break loop
		default:
			// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
			if err = triggerSourceCmdProbe(probe, execCommandDetails, clients, chaosresult); err != nil {
				err = addProbePhase(err, string(chaosDetails.Phase))
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
						chaosresult.ProbeDetails[index].Status.Description = getDescription(err)
						log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
						isExperimentFailed = true
						break loop
					}
				}
			}
			// waiting for the probe polling interval
			time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
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

// triggerSourceContinuousCmdProbe trigger the continuous cmd probes having need some external source image
func triggerSourceContinuousCmdProbe(probe v1alpha1.ProbeAttributes, execCommandDetails litmusexec.PodDetails, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) {

	var isExperimentFailed bool
	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
	}

	// it trigger the cmd probe for the entire duration of chaos and it fails, if any err encounter
	// it marked the error for the probes, if any
loop:
	for {
		err = triggerSourceCmdProbe(probe, execCommandDetails, clients, chaosresult)
		// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
		if err != nil {
			err = addProbePhase(err, string(chaosDetails.Phase))
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
					chaosresult.ProbeDetails[index].Status.Description = getDescription(err)
					log.Errorf("The %v cmd probe has been Failed, err: %v", probe.Name, err)
					isExperimentFailed = true
					break loop
				}
			}
		}
		// waiting for the probe polling interval
		time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
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

// validateResult validate the probe result to specified comparison operation
// it supports int, float, string operands
func validateResult(comparator v1alpha1.ComparatorInfo, probeName, cmdOutput string, rc int) (string, error) {

	compare := cmp.RunCount(rc).
		FirstValue(cmdOutput).
		SecondValue(comparator.Value).
		Criteria(comparator.Criteria).
		ProbeName(probeName)

	switch strings.ToLower(comparator.Type) {
	case "int":
		if err = compare.CompareInt(cerrors.ErrorTypeCmdProbe); err != nil {
			return "", err
		}
	case "float":
		if err = compare.CompareFloat(cerrors.ErrorTypeCmdProbe); err != nil {
			return "", err
		}
	case "string":
		if err = compare.CompareString(cerrors.ErrorTypeCmdProbe); err != nil {
			return "", err
		}
	default:
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{name: %v}", probeName), Reason: fmt.Sprintf("comparator type '%s' not supported in the cmd probe", comparator.Type)}
	}
	description := fmt.Sprintf("Probe responded with a valid output. Actual and Expected values are '%s' and '%s' respectively", cmdOutput, comparator.Value)
	return description, nil
}

// preChaosCmdProbe trigger the cmd probe for prechaos phase
func preChaosCmdProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

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
		if reflect.DeepEqual(probe.CmdProbeInputs.Source, v1alpha1.SourceDetails{}) {
			err = triggerInlineCmdProbe(probe, resultDetails)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			// it will update the status of all the unrun probes as well
			if err := markedVerdictInEnd(err, resultDetails, probe, "PreChaos"); err != nil {
				return err
			}
		} else {

			execCommandDetails, err := createHelperPod(probe, resultDetails, clients, chaosDetails)
			if err != nil {
				return err
			}

			// triggering the cmd probe and storing the output into the out buffer
			err = triggerSourceCmdProbe(probe, execCommandDetails, clients, resultDetails)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			// it will update the status of all the unrun probes as well
			if err = markedVerdictInEnd(err, resultDetails, probe, "PreChaos"); err != nil {
				return err
			}

			// get runId
			runID := getRunIDFromProbe(resultDetails, probe.Name, probe.Type)

			// deleting the external pod which was created for cmd probe
			if err = deleteProbePod(chaosDetails, clients, runID, probe.Name); err != nil {
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
		if reflect.DeepEqual(probe.CmdProbeInputs.Source, v1alpha1.SourceDetails{}) {
			go triggerInlineContinuousCmdProbe(probe, clients, resultDetails, chaosDetails)
		} else {

			execCommandDetails, err := createHelperPod(probe, resultDetails, clients, chaosDetails)
			if err != nil {
				return err
			}

			// trigger the continuous cmd probe
			go triggerSourceContinuousCmdProbe(probe, execCommandDetails, clients, resultDetails, chaosDetails)
		}

	}
	return nil
}

// postChaosCmdProbe trigger cmd probe for post chaos phase
func postChaosCmdProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

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
		if reflect.DeepEqual(probe.CmdProbeInputs.Source, v1alpha1.SourceDetails{}) {
			err = triggerInlineCmdProbe(probe, resultDetails)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			// it will update the status of all the unrun probes as well
			if err = markedVerdictInEnd(err, resultDetails, probe, "PostChaos"); err != nil {
				return err
			}
		} else {

			execCommandDetails, err := createHelperPod(probe, resultDetails, clients, chaosDetails)
			if err != nil {
				return err
			}

			// triggering the cmd probe and storing the output into the out buffer
			err = triggerSourceCmdProbe(probe, execCommandDetails, clients, resultDetails)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			// it will update the status of all the unrun probes as well
			if err = markedVerdictInEnd(err, resultDetails, probe, "PostChaos"); err != nil {
				return err
			}

			// get runId
			runID := getRunIDFromProbe(resultDetails, probe.Name, probe.Type)

			// deleting the external pod which was created for cmd probe
			if err = deleteProbePod(chaosDetails, clients, runID, probe.Name); err != nil {
				return err
			}
		}
	case "Continuous", "OnChaos":
		if reflect.DeepEqual(probe.CmdProbeInputs.Source, v1alpha1.SourceDetails{}) {
			// it will check for the error, It will detect the error if any error encountered in probe during chaos
			err = checkForErrorInContinuousProbe(resultDetails, probe.Name)
			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			if err = markedVerdictInEnd(err, resultDetails, probe, "PostChaos"); err != nil {
				return err
			}
		} else {
			// it will check for the error, It will detect the error if any error encountered in probe during chaos
			err = checkForErrorInContinuousProbe(resultDetails, probe.Name)

			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			if err = markedVerdictInEnd(err, resultDetails, probe, "PostChaos"); err != nil {
				return err
			}
			// get runId
			runID := getRunIDFromProbe(resultDetails, probe.Name, probe.Type)
			// deleting the external pod, which was created for cmd probe
			if err = deleteProbePod(chaosDetails, clients, runID, probe.Name); err != nil {
				return err
			}

		}
	}
	return nil
}

// onChaosCmdProbe trigger the cmd probe for DuringChaos phase
func onChaosCmdProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

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
		if reflect.DeepEqual(probe.CmdProbeInputs.Source, v1alpha1.SourceDetails{}) {
			go triggerInlineOnChaosCmdProbe(probe, clients, resultDetails, chaosDetails)
		} else {

			execCommandDetails, err := createHelperPod(probe, resultDetails, clients, chaosDetails)
			if err != nil {
				return err
			}
			// trigger the continuous cmd probe
			go triggerSourceOnChaosCmdProbe(probe, execCommandDetails, clients, resultDetails, chaosDetails)
		}

	}
	return nil
}

// createHelperPod create the helper pod with the source image
// it will be created if the mode is not inline
func createHelperPod(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (litmusexec.PodDetails, error) {
	// Generate the run_id
	runID := stringutils.GetRunID()
	setRunIDForProbe(resultDetails, probe.Name, probe.Type, runID)

	// create the external pod with source image for cmd probe
	if err := createProbePod(clients, chaosDetails, runID, probe.CmdProbeInputs.Source, probe.Name); err != nil {
		return litmusexec.PodDetails{}, err
	}

	// verify the running status of external probe pod
	log.Info("[Status]: Checking the status of the probe pod")
	if err = status.CheckApplicationStatusesByLabels(chaosDetails.ChaosNamespace, "name="+chaosDetails.ExperimentName+"-probe-"+runID, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return litmusexec.PodDetails{}, stacktrace.Propagate(err, "probe pod is not in running state")
	}

	// setting the attributes for the exec command
	execCommandDetails := litmusexec.PodDetails{}
	litmusexec.SetExecCommandAttributes(&execCommandDetails, chaosDetails.ExperimentName+"-probe-"+runID, chaosDetails.ExperimentName+"-probe", chaosDetails.ChaosNamespace)

	return execCommandDetails, nil
}

// getServiceAccount derive the serviceAccountName for the probe pod
func getServiceAccount(chaosNamespace, chaosPodName, probeName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(chaosNamespace).Get(context.Background(), chaosPodName, v1.GetOptions{})
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeCmdProbe, Target: fmt.Sprintf("{name: %v}", probeName), Reason: err.Error()}
	}
	return pod.Spec.ServiceAccountName, nil
}
