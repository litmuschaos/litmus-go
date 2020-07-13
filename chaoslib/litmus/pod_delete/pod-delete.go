package pod_delete

import (
	"math/rand"
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//PreparePodDelete contains the prepration steps before chaos injection
func PreparePodDelete(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	//Getting the iteration count for the pod deletion
	GetIterations(experimentsDetails)
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}

	// Getting the serviceAccountName
	err = GetServiceAccount(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the serviceAccountName, err: %v", err)
	}

	// Generate the run_id
	runID := GetRunID()

	// Creating a helper pod
	err = CreateHelperPod(experimentsDetails, clients, runID)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name=pod-delete"+runID, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Recording the chaos start timestamp
	ChaosStartTimeStamp := time.Now().Unix()

	// Wait till the completion of helper pod
	log.Info("[Wait]: waiting till the completion of the helper pod")
	err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "name=pod-delete"+runID, clients, experimentsDetails.ChaosDuration+experimentsDetails.ChaosInterval+60)
	if err != nil {
		return err
	}

	//ChaosCurrentTimeStamp contains the current timestamp
	ChaosCurrentTimeStamp := time.Now().Unix()
	//ChaosDiffTimeStamp contains the difference of current timestamp and start timestamp
	//It will helpful to track the total chaos duration
	chaosDiffTimeStamp := ChaosCurrentTimeStamp - ChaosStartTimeStamp

	if int(chaosDiffTimeStamp) < experimentsDetails.ChaosDuration {
		return errors.Errorf("The helper pod failed, check the logs of helper pod for more details")
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = DeleteHelperPod(experimentsDetails, clients, runID)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}
	return nil
}

//GetIterations derive the iterations value from given parameters
func GetIterations(experimentsDetails *experimentTypes.ExperimentDetails) {
	var Iterations int
	if experimentsDetails.ChaosInterval != 0 {
		Iterations = experimentsDetails.ChaosDuration / experimentsDetails.ChaosInterval
	} else {
		Iterations = 0
	}
	experimentsDetails.Iterations = math.Maximum(Iterations, 1)

}

//waitForRampTime waits for the given ramp time duration (in seconds)
func waitForRampTime(experimentsDetails *experimentTypes.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.RampTime) * time.Second)
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

// GetServiceAccount find the serviceAccountName for the helper pod
func GetServiceAccount(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Get(experimentsDetails.ChaosPodName, v1.GetOptions{})
	if err != nil {
		return err
	}
	experimentsDetails.ChaosServiceAccount = pod.Spec.ServiceAccountName
	return nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, runID string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod-delete-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      "pod-delete",
				"name":     "pod-delete" + runID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
		},
		Spec: apiv1.PodSpec{
			ServiceAccountName: experimentsDetails.ChaosServiceAccount,
			RestartPolicy:      apiv1.RestartPolicyNever,
			Containers: []apiv1.Container{
				{
					Name:            "pod-delete",
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullAlways,
					Command: []string{
						"bin/bash",
					},
					Args: []string{
						"-c",
						"./experiments/pod-delete",
					},
					Env: GetPodEnv(experimentsDetails),
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err

}

//DeleteHelperPod delete the helper pod
func DeleteHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, runID string) error {

	err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Delete("pod-delete-"+runID, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: "name=pod-delete" + runID})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Pod is not deleted yet, err: %v", err)
			}
			return nil
		})

	return err
}

// GetPodEnv derive all the env required for the helper pod
func GetPodEnv(experimentsDetails *experimentTypes.ExperimentDetails) []apiv1.EnvVar {

	var envVar []apiv1.EnvVar
	ENVList := map[string]string{
		"FORCE":                strconv.FormatBool(experimentsDetails.Force),
		"APP_NS":               experimentsDetails.AppNS,
		"KILL_COUNT":           strconv.Itoa(experimentsDetails.KillCount),
		"TOTAL_CHAOS_DURATION": strconv.Itoa(experimentsDetails.ChaosDuration),
		"CHAOS_NAMESPACE":      experimentsDetails.ChaosNamespace,
		"APP_LABEL":            experimentsDetails.AppLabel,
		"CHAOS_ENGINE":         experimentsDetails.EngineName,
		"CHAOS_UID":            string(experimentsDetails.ChaosUID),
		"CHAOS_INTERVAL":       strconv.Itoa(experimentsDetails.ChaosInterval),
		"ITERATIONS":           strconv.Itoa(experimentsDetails.Iterations),
	}
	for key, value := range ENVList {
		var perEnv apiv1.EnvVar
		perEnv.Name = key
		perEnv.Value = value
		envVar = append(envVar, perEnv)
	}
	// Getting experiment pod name from downward API
	experimentPodName := GetValueFromDownwardAPI("v1", "metadata.name")

	var downwardEnv apiv1.EnvVar
	downwardEnv.Name = "POD_NAME"
	downwardEnv.ValueFrom = &experimentPodName
	envVar = append(envVar, downwardEnv)

	return envVar
}

// GetValueFromDownwardAPI returns the value from downwardApi
func GetValueFromDownwardAPI(apiVersion string, fieldPath string) apiv1.EnvVarSource {
	downwardENV := apiv1.EnvVarSource{
		FieldRef: &apiv1.ObjectFieldSelector{
			APIVersion: apiVersion,
			FieldPath:  fieldPath,
		},
	}
	return downwardENV
}
