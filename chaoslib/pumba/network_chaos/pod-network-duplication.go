package network_chaos

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/environment"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog"
)

var err error

//PreparePodNetworkDuplication contains the prepration steps before chaos injection
func PreparePodNetworkDuplication(experimentsDetails *types.ExperimentDetails, clients environment.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	//Select application pod for pod network duplication
	appName, appNodeName, err := GetApplicationPod(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the application name and application nodename due to, err: %v", err)
	}
	log.Infof("[Prepare]: Application pod name under chaos: %v", appName)
	log.Infof("[Prepare]: Application node name: %v", appNodeName)

	//Get the target contaien name of the application pod
	var targetContainer string
	if experimentsDetails.TargetContainer == "" {
		targetContainer, err = GetTargetContainer(experimentsDetails, appName, clients)
		if err != nil {
			return errors.Errorf("Unable to get the target container name due to, err: %v", err)
		}
		log.Infof("[Prepare]: Target container name: %v", targetContainer)
	}

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

	// Creating the helper pod to perform pod network duplication
	err = CreateHelperPod(experimentsDetails, clients, targetContainer, runID, appName, appNodeName)
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
	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration+30) * time.Second

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
loop:
	for {
		endTime = time.After(timeDelay)
		select {
		case <-signChan:
			log.Info("[Chaos]: Killing process started because of terminated signal received")
			err = KillNetworkDuplication(targetContainer, appName, experimentsDetails.AppNS, clients)
			if err != nil {
				klog.V(0).Infof("Error in Kill stress after")
				return err
			}
			resultDetails.FailStep = "Chaos injection stopped!"
			resultDetails.Verdict = "Stopped"
			result.ChaosResult(experimentsDetails, clients, resultDetails, "EOT")
			os.Exit(1)
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			endTime = nil
			break loop
		}
	}

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
func GetApplicationPod(experimentsDetails *types.ExperimentDetails, clients environment.ClientSets) (string, string, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil {
		return "", "", err
	}

	podNameListSize := len(podList.Items) + 1
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
func GetTargetContainer(experimentsDetails *types.ExperimentDetails, appName string, clients environment.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "Fail to get the application pod status, due to:%v", err)
	}

	return pod.Spec.Containers[0].Name, nil
}

//waitForRampTime waits for the given ramp time duration (in seconds)
func waitForRampTime(experimentsDetails *types.ExperimentDetails) {
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
func GetServiceAccount(experimentsDetails *types.ExperimentDetails, clients environment.ClientSets) error {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Get(experimentsDetails.ChaosPodName, v1.GetOptions{})
	if err != nil {
		return err
	}
	experimentsDetails.ChaosServiceAccount = pod.Spec.ServiceAccountName
	return nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *types.ExperimentDetails, clients environment.ClientSets, targetContainer string, runID string, appName string, appNodeName string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pumba-netem-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      "pumba-netem",
				"name":     "pumba-netem-" + runID,
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
						"re2:k8s_" + targetContainer + "_" + appName,
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
func DeleteHelperPod(experimentsDetails *types.ExperimentDetails, clients environment.ClientSets, runID string) error {

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

// KillNetworkDuplication is a Function to kill the experiment. Triggered by either timeout of chaos duration or termination of the experiment
func KillNetworkDuplication(containerName, podName, namespace string, clients environment.ClientSets) error {

	command := []string{"/bin/sh", "-c", "kill $(find /proc -name exe -lname '*/netem' 2>&1 | grep -v 'Permission denied' | awk -F/ '{print $(NF-1)}' |  head -n 1)"}

	req := clients.KubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")
	scheme := runtime.NewScheme()
	if err := apiv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("error adding to scheme: %v", err)
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&apiv1.PodExecOptions{
		Command:   command,
		Container: containerName,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(clients.KubeConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("error while creating Executor: %v", err)
	}

	stdout := os.Stdout
	stderr := os.Stderr

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})

	//The kill command returns a 143 when it kills a process. This is expected
	if err != nil {
		error_code := strings.Contains(err.Error(), "143")
		if error_code != true {
			log.Infof("[Chaos]:Pod Network Duplication error: %v", err.Error())
			return err
		}
	}

	return nil
}
