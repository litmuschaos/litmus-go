package lib

import (
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PreparePodDelete contains the prepration steps before chaos injection
func PreparePodDelete(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.ChaosServiceAccount == "" {
		// Getting the serviceAccountName for the powerfulseal pod
		err := GetServiceAccount(experimentsDetails, clients)
		if err != nil {
			return errors.Errorf("Unable to get the serviceAccountName, err: %v", err)
		}
	}

	// generating a unique string which can be appended with the powerfulseal deployment name & labels for the uniquely identification
	runID := common.GetRunID()

	// generating the chaos inject event in the chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// Creating configmap for powerfulseal deployment
	err := CreateConfigMap(experimentsDetails, clients, runID)
	if err != nil {
		return err
	}

	// Creating powerfulseal deployment
	err = CreatePowerfulsealDeployment(experimentsDetails, clients, runID)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//checking the status of the powerfulseal pod, wait till the powerfulseal pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name=powerfulseal-"+runID, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("powerfulseal pod is not in running state, err: %v", err)
	}

	// Wait for Chaos Duration
	log.Infof("[Wait]: Waiting for the %vs chaos duration", experimentsDetails.ChaosDuration)
	common.WaitForDuration(experimentsDetails.ChaosDuration)

	//Deleting the powerfulseal deployment
	log.Info("[Cleanup]: Deleting the powerfulseal deployment")
	err = DeletePowerfulsealDeployment(experimentsDetails, clients, runID)
	if err != nil {
		return errors.Errorf("Unable to delete the  powerfulseal deployment, err: %v", err)
	}

	//Deleting the powerfulseal configmap
	log.Info("[Cleanup]: Deleting the powerfulseal configmap")
	err = DeletePowerfulsealConfigmap(experimentsDetails, clients, runID)
	if err != nil {
		return errors.Errorf("Unable to delete the  powerfulseal configmap, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// GetServiceAccount find the serviceAccountName for the powerfulseal deployment
func GetServiceAccount(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Get(experimentsDetails.ChaosPodName, v1.GetOptions{})
	if err != nil {
		return err
	}
	experimentsDetails.ChaosServiceAccount = pod.Spec.ServiceAccountName
	return nil
}

// CreateConfigMap creates a configmap for the powerfulseal deployment
func CreateConfigMap(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, runID string) error {

	data := map[string]string{}

	// It will store all the details inside a string in well formated way
	policy := GetConfigMapData(experimentsDetails)

	data["policy"] = policy
	configMap := &apiv1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "policy-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"name": "policy-" + runID,
			},
		},
		Data: data,
	}

	_, err := clients.KubeClient.CoreV1().ConfigMaps(experimentsDetails.ChaosNamespace).Create(configMap)

	return err
}

// GetConfigMapData generates the configmap data for the powerfulseal deployments in desired format format
func GetConfigMapData(experimentsDetails *experimentTypes.ExperimentDetails) string {

	waitTime, _ := strconv.Atoi(experimentsDetails.ChaosInterval)
	policy := "config:" + "\n" +
		"  minSecondsBetweenRuns: 1" + "\n" +
		"  maxSecondsBetweenRuns: " + strconv.Itoa(waitTime) + "\n" +
		"podScenarios:" + "\n" +
		"  - name: \"delete random pods in application namespace\"" + "\n" +
		"    match:" + "\n" +
		"      - labels:" + "\n" +
		"          namespace: " + experimentsDetails.AppNS + "\n" +
		"          selector: " + experimentsDetails.AppLabel + "\n" +
		"    filters:" + "\n" +
		"      - randomSample:" + "\n" +
		"          size: 1" + "\n" +
		"    actions:" + "\n" +
		"      - kill:" + "\n" +
		"          probability: 0.77" + "\n" +
		"          force: " + strconv.FormatBool(experimentsDetails.Force)

	return policy

}

// CreatePowerfulsealDeployment derive the attributes for powerfulseal deployment and create it
func CreatePowerfulsealDeployment(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, runID string) error {

	deployment := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      "powerfulseal-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       "powerfulseal",
				"name":                      "powerfulseal-" + runID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"name":     "powerfulseal-" + runID,
					"chaosUID": string(experimentsDetails.ChaosUID),
				},
			},
			Replicas: func(i int32) *int32 { return &i }(1),
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"name":     "powerfulseal-" + runID,
						"chaosUID": string(experimentsDetails.ChaosUID),
					},
				},
				Spec: apiv1.PodSpec{
					Volumes: []apiv1.Volume{
						{
							Name: "policyfile",
							VolumeSource: apiv1.VolumeSource{
								ConfigMap: &apiv1.ConfigMapVolumeSource{
									LocalObjectReference: apiv1.LocalObjectReference{
										Name: "policy-" + runID,
									},
								},
							},
						},
					},
					ServiceAccountName:            experimentsDetails.ChaosServiceAccount,
					TerminationGracePeriodSeconds: func(i int64) *int64 { return &i }(0),
					Containers: []apiv1.Container{
						{
							Name:  "powerfulseal",
							Image: "ksatchit/miko-powerfulseal:non-ssh",
							Args: []string{
								"autonomous",
								"--inventory-kubernetes",
								"--no-cloud",
								"--policy-file=/root/policy_kill_random_default.yml",
								"--use-pod-delete-instead-of-ssh-kill",
							},
							VolumeMounts: []apiv1.VolumeMount{
								{
									Name:      "policyfile",
									MountPath: "/root/policy_kill_random_default.yml",
									SubPath:   "policy",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.ChaosNamespace).Create(deployment)
	return err

}

//DeletePowerfulsealDeployment delete the powerfulseal deployment
func DeletePowerfulsealDeployment(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, runID string) error {

	err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.ChaosNamespace).Delete("powerfulseal-"+runID, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: "name=powerfulseal-" + runID})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Deployment is not deleted yet, err: %v", err)
			}
			return nil
		})

	return err
}

//DeletePowerfulsealConfigmap delete the powerfulseal configmap
func DeletePowerfulsealConfigmap(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, runID string) error {

	err := clients.KubeClient.CoreV1().ConfigMaps(experimentsDetails.ChaosNamespace).Delete("policy-"+runID, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().ConfigMaps(experimentsDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: "name=policy-" + runID})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("configmap is not deleted yet, err: %v", err)
			}
			return nil
		})

	return err
}
