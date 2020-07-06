package network_chaos

import (
	"math/rand"
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-network-duplication/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//PreparePodNetworkDuplication contains the prepration steps before chaos injection
func PreparePodNetworkDuplication(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	//Select application pod for pod network duplication
	appName, appNodeName, err := GetApplicationPod(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the application name and application nodename due to, err: %v", err)
	}
	log.Infof("[Prepare]: Application pod name under chaos: %v", appName)
	log.Infof("[Prepare]: Application node name: %v", appNodeName)

	//Get the target container name of the application pod
	experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, appName, clients)
	if err != nil {
		return errors.Errorf("Unable to get the target container name due to, err: %v", err)
	}
	log.Infof("[Prepare]: Target container name: %v", experimentsDetails.TargetContainer)

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
	experimentsDetails.RunID = runID

	// Creating the helper pod to perform pod network duplication
	err = CreateHelperPod(experimentsDetails, clients, appName, appNodeName)
	if err != nil {
		errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name=pumba-netem-"+runID, clients)
	if err != nil {
		errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Info("[Wait]: waiting till the completion of the helper pod")

	err = status.WaitForCompletion(experimentsDetails.ChaosNamespace, "name=pumba-netem-"+runID, clients, experimentsDetails.ChaosDuration+30)
	if err != nil {
		return err
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = DeleteHelperPod(experimentsDetails, clients, runID)
	if err != nil {
		errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}
	return nil
}

//GetApplicationPod will select a random replica of application pod for chaos
//It will also get the node name of the application pod
func GetApplicationPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, string, error) {
	podList, _ := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if len(podList.Items) == 0 {
		return "", "", errors.Wrapf(err, "Fail to get the application pod in %v namespace", experimentsDetails.AppNS)
	}

	podNameListSize := len(podList.Items)
	podNameList := make([]string, podNameListSize)
	podNodeName := make([]string, podNameListSize)

	for i, pod := range podList.Items {
		podNameList[i] = pod.Name
		podNodeName[i] = pod.Spec.NodeName
	}

	rand.Seed(time.Now().Unix())
	randomIndex := rand.Intn(len(podNameList))
	applicationName := podNameList[randomIndex]
	nodeName := podNodeName[randomIndex]

	return applicationName, nodeName, nil
}

//GetTargetContainer will fetch the conatiner name from application pod
//This container will be used as target container
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "Fail to get the application pod status, due to:%v", err)
	}

	return pod.Spec.Containers[0].Name, nil
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
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName, appNodeName string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pumba-netem-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      "pumba",
				"name":     "pumba-netem-" + experimentsDetails.RunID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
		},
		Spec: apiv1.PodSpec{
			ServiceAccountName: experimentsDetails.ChaosServiceAccount,
			RestartPolicy:      apiv1.RestartPolicyNever,
			NodeName:           appNodeName,
			Volumes: []apiv1.Volume{
				apiv1.Volume{
					Name: "dockersocket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: "/var/run/docker.sock",
						},
					},
				},
			},
			Containers: []apiv1.Container{
				{
					Name:            "pumba",
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullAlways,
					Args: []string{
						"netem",
						"--tc-image",
						"gaiadocker/iproute2",
						"--interface",
						experimentsDetails.NetworkInterface,
						"--duration",
						strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
						"duplicate",
						"--percent",
						strconv.Itoa(experimentsDetails.NetworkPacketDuplicationPercentage),
						"re2:k8s_" + experimentsDetails.TargetContainer + "_" + appName,
					},
					VolumeMounts: []apiv1.VolumeMount{
						apiv1.VolumeMount{
							Name:      "dockersocket",
							MountPath: "/var/run/docker.sock",
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}

//DeleteHelperPod delete the helper pod
func DeleteHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, runID string) error {

	err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Delete("pumba-netem-"+runID, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: "name=pumba-netem-" + runID})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Helper Pod is not deleted yet, err: %v", err)
			}
			return nil
		})

	return err
}
