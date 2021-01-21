package lib

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//PrepareAndInjectChaos contains the prepration & injection steps
func PrepareAndInjectChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, args string) error {

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	targetPodList, err := common.GetPodList(experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
	if err != nil {
		return err
	}

	podNames := []string{}
	for _, pod := range targetPodList.Items {
		podNames = append(podNames, pod.Name)
	}

	log.Infof("Target pods list for chaos, %v", podNames)

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// Getting the serviceAccountName, need permission inside helper pod to create the events
	if experimentsDetails.ChaosServiceAccount == "" {
		err = GetServiceAccount(experimentsDetails, clients)
		if err != nil {
			return errors.Errorf("Unable to get the serviceAccountName, err: %v", err)
		}
	}

	//Get the target container name of the application pod
	if experimentsDetails.TargetContainer == "" {
		experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, targetPodList.Items[0].Name, clients)
		if err != nil {
			return errors.Errorf("Unable to get the target container name, err: %v", err)
		}
	}

	if experimentsDetails.EngineName != "" {
		// Get Chaos Pod Annotation
		experimentsDetails.Annotations, err = common.GetChaosPodAnnotation(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("unable to get annotations, err: %v", err)
		}
		// Get Resource Requirements
		experimentsDetails.Resources, err = common.GetChaosPodResourceRequirements(experimentsDetails.ChaosPodName, experimentsDetails.ExperimentName, experimentsDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("Unable to get resource requirements, err: %v", err)
		}
	}

	if experimentsDetails.Sequence == "serial" {
		if err = InjectChaosInSerialMode(experimentsDetails, targetPodList, clients, chaosDetails, args, resultDetails); err != nil {
			return err
		}
	} else {
		if err = InjectChaosInParallelMode(experimentsDetails, targetPodList, clients, chaosDetails, args, resultDetails); err != nil {
			return err
		}
	}
	return nil
}

// InjectChaosInSerialMode inject the network chaos in all target application serially (one by one)
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, args string, resultDetails *types.ResultDetails) error {

	// creating the helper pod to perform network chaos
	for _, pod := range targetPodList.Items {

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":       pod.Name,
			"NodeName":      pod.Spec.NodeName,
			"ContainerName": experimentsDetails.TargetContainer,
		})
		runID := common.GetRunID()
		err = CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID, args, "inject")
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}

		//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
		log.Info("[Status]: Checking the status of the helper pods")
		err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("helper pods are not in running state, err: %v", err)
		}

		// Wait till the completion of the helper pod
		// set an upper limit for the waiting time
		log.Info("[Wait]: waiting till the completion of the helper pod")
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", clients, experimentsDetails.ChaosDuration+60, experimentsDetails.ExperimentName)
		if err != nil || podStatus == "Failed" {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			recoveryErr := ChaosRecovery(resultDetails, chaosDetails, experimentsDetails, clients, args)
			if updateErr := updateChaosStatus(resultDetails, chaosDetails, clients); err != nil {
				return updateErr
			}
			if recoveryErr != nil {
				return recoveryErr
			}
			return errors.Errorf("helper pod failed due to, err: %v", err)
		}

		//Deleting all the helper pod for container-kill chaos
		log.Info("[Cleanup]: Deleting the the helper pod")
		err = common.DeletePod(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
		if err != nil {
			return errors.Errorf("Unable to delete the helper pods, err: %v", err)
		}
	}

	return nil
}

// InjectChaosInParallelMode inject the network chaos in all target application in parallel mode (all at once)
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, args string, resultDetails *types.ResultDetails) error {

	// creating the helper pod to perform network chaos
	for _, pod := range targetPodList.Items {

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":       pod.Name,
			"NodeName":      pod.Spec.NodeName,
			"ContainerName": experimentsDetails.TargetContainer,
		})
		runID := common.GetRunID()
		err = CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID, args, "inject")
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}
	}

	//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pods")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
		return errors.Errorf("helper pods are not in running state, err: %v", err)
	}

	// Wait till the completion of the helper pod
	// set an upper limit for the waiting time
	log.Info("[Wait]: waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", clients, experimentsDetails.ChaosDuration+60, experimentsDetails.ExperimentName)
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
		recoveryErr := ChaosRecovery(resultDetails, chaosDetails, experimentsDetails, clients, args)
		if updateErr := updateChaosStatus(resultDetails, chaosDetails, clients); err != nil {
			return updateErr
		}
		if recoveryErr != nil {
			return recoveryErr
		}
		return errors.Errorf("helper pod failed due to, err: %v", err)
	}

	//Deleting all the helper pod for container-kill chaos
	log.Info("[Cleanup]: Deleting all the helper pod")
	err = common.DeleteAllPod("app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pods, err: %v", err)
	}

	return nil
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

//GetTargetContainer will fetch the container name from application pod
//This container will be used as target container
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", err
	}

	return pod.Spec.Containers[0].Name, nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, podName, nodeName, runID, args, chaosType string) error {

	privilegedEnable := true

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       experimentsDetails.ExperimentName + "-helper",
				"name":                      experimentsDetails.ExperimentName + "-" + runID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
			},
			Annotations: experimentsDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			HostPID:            true,
			ServiceAccountName: experimentsDetails.ChaosServiceAccount,
			RestartPolicy:      apiv1.RestartPolicyNever,
			NodeName:           nodeName,
			Volumes: []apiv1.Volume{
				{
					Name: "cri-socket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: experimentsDetails.SocketPath,
						},
					},
				},
			},
			InitContainers: []apiv1.Container{
				{
					Name:            "setup-" + experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"/bin/bash",
						"-c",
						"sudo chmod 777 " + experimentsDetails.SocketPath,
					},
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "cri-socket",
							MountPath: experimentsDetails.SocketPath,
						},
					},
				},
			},

			Containers: []apiv1.Container{
				{
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"/bin/bash",
					},
					Args: []string{
						"-c",
						"./helper/network-chaos",
					},
					Resources: experimentsDetails.Resources,
					Env:       GetPodEnv(experimentsDetails, podName, args, chaosType),
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "cri-socket",
							MountPath: experimentsDetails.SocketPath,
						},
					},
					SecurityContext: &apiv1.SecurityContext{
						Privileged: &privilegedEnable,
						Capabilities: &apiv1.Capabilities{
							Add: []apiv1.Capability{
								"NET_ADMIN",
								"SYS_ADMIN",
							},
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err

}

// GetPodEnv derive all the env required for the helper pod
func GetPodEnv(experimentsDetails *experimentTypes.ExperimentDetails, podName, args, chaosType string) []apiv1.EnvVar {

	var envVar []apiv1.EnvVar
	ENVList := map[string]string{
		"APP_NS":               experimentsDetails.AppNS,
		"APP_POD":              podName,
		"APP_CONTAINER":        experimentsDetails.TargetContainer,
		"TOTAL_CHAOS_DURATION": strconv.Itoa(experimentsDetails.ChaosDuration),
		"CHAOS_NAMESPACE":      experimentsDetails.ChaosNamespace,
		"CHAOS_ENGINE":         experimentsDetails.EngineName,
		"CHAOS_UID":            string(experimentsDetails.ChaosUID),
		"CONTAINER_RUNTIME":    experimentsDetails.ContainerRuntime,
		"NETEM_COMMAND":        args,
		"NETWORK_INTERFACE":    experimentsDetails.NetworkInterface,
		"EXPERIMENT_NAME":      experimentsDetails.ExperimentName,
		"SOCKET_PATH":          experimentsDetails.SocketPath,
		"DESTINATION_IPS":      GetTargetIpsArgs(experimentsDetails.DestinationIPs, experimentsDetails.DestinationHosts),
		"CHAOS_TYPE":           chaosType,
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

// GetTargetIpsArgs return the comma separated target ips
// It fetch the ips from the target ips (if defined by users)
// it append the ips from the host, if target host is provided
func GetTargetIpsArgs(targetIPs, targetHosts string) string {

	ipsFromHost := GetIpsForTargetHosts(targetHosts)
	if targetIPs == "" {
		targetIPs = ipsFromHost
	} else if ipsFromHost != "" {
		targetIPs = targetIPs + "," + ipsFromHost
	}
	return targetIPs
}

// GetIpsForTargetHosts resolves IP addresses for comma-separated list of target hosts and returns comma-separated ips
func GetIpsForTargetHosts(targetHosts string) string {
	if targetHosts == "" {
		return ""
	}
	hosts := strings.Split(targetHosts, ",")
	var commaSeparatedIPs []string
	for i := range hosts {
		ips, err := net.LookupIP(hosts[i])
		if err != nil {
			log.Infof("Unknown host")
		} else {
			for j := range ips {
				log.Infof("IP address: %v", ips[j])
				commaSeparatedIPs = append(commaSeparatedIPs, ips[j].String())
			}
		}
	}
	return strings.Join(commaSeparatedIPs, ",")
}

// updateChaosStatus ...
func updateChaosStatus(resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, clients clients.ClientSets) error {

	result, err := clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Get(resultDetails.Name, v1.GetOptions{})
	if err != nil {
		return err
	}
	annotations := result.ObjectMeta.Annotations
	chaosStatusList := []v1alpha1.ChaosStatusDetails{}
	for k, v := range annotations {
		podName := strings.TrimSpace(strings.Split(k, "/")[1])
		chaosStatus := v1alpha1.ChaosStatusDetails{
			TargetPodName: podName,
			ChaosStatus:   v,
		}
		chaosStatusList = append(chaosStatusList, chaosStatus)
		delete(annotations, k)
	}

	result.Status.History.ChaosStatus = chaosStatusList
	result.ObjectMeta.Annotations = annotations
	_, err = clients.LitmusClient.ChaosResults(chaosDetails.ChaosNamespace).Update(result)
	return err
}

// getTargetPodsForRecovery derive the name of target pods for recovery
// pods which contains chaosStatus as injected
func getTargetPodsForRecovery(resultName, namespace string, clients clients.ClientSets) ([]string, error) {
	result, err := clients.LitmusClient.ChaosResults(namespace).Get(resultName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	chaosStatusList := result.ObjectMeta.Annotations
	targetPodForRecovery := []string{}
	for k, v := range chaosStatusList {
		if strings.ToLower(v) == "injected" {
			podName := strings.TrimSpace(strings.Split(k, "/")[1])
			if podName == "" {
				return nil, errors.Errorf("Wrong formated annotations found inside %v chaosresult", result.Name)
			}
			targetPodForRecovery = append(targetPodForRecovery, podName)
		}
	}
	return targetPodForRecovery, nil
}

// ChaosRecovery contains steps for the chaos recovery
func ChaosRecovery(resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, args string) error {

	return retry.
		Times(uint(3)).
		Wait(time.Duration(2) * time.Second).
		Try(func(attempt uint) error {
			targetPods, err := getTargetPodsForRecovery(resultDetails.Name, chaosDetails.ChaosNamespace, clients)
			if err != nil {
				return err
			}
			if len(targetPods) == 0 {
				log.Info("No target pod found for recovery!")
				return nil
			}
			for _, pod := range targetPods {
				nodeName, err := getNodeName(pod, experimentsDetails.AppNS, clients)
				if err != nil {
					return err
				}
				runID := common.GetRunID()
				if err := CreateHelperPod(experimentsDetails, clients, pod, nodeName, runID, args, "recover"); err != nil {
					return err
				}
				// Wait till the completion of the helper pod
				// set an upper limit for the waiting time
				log.Info("[Wait]: waiting till the completion of the helper pod")
				podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", clients, 60, experimentsDetails.ExperimentName)
				if err != nil || podStatus == "Failed" {
					common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
					return errors.Errorf("helper pod failed due to, err: %v", err)
				}
			}
			return nil
		})
}

// getNodeName derive the node name of the pod
func getNodeName(podName, namespace string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(namespace).Get(podName, v1.GetOptions{})
	if err != nil {
		return "", err
	}
	return pod.Spec.NodeName, nil
}
