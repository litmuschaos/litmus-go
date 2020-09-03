package common

import (
	"math/rand"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//WaitForDuration waits for the given time duration (in seconds)
func WaitForDuration(duration int) {
	time.Sleep(time.Duration(duration) * time.Second)
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

//DeletePod deletes the specified pod and wait until it got terminated
func DeletePod(podName, podLabel, namespace string, timeout, delay int, clients clients.ClientSets) error {

	err := clients.KubeClient.CoreV1().Pods(namespace).Delete(podName, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	// waiting for the termination of the pod
	err = retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: podLabel})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Unable to delete the pod, err: %v", err)
			}
			return nil
		})

	return err
}

// CheckForAvailibiltyOfPod check the availibility of the specified pod
func CheckForAvailibiltyOfPod(namespace, name string, clients clients.ClientSets) (bool, error) {

	if name == "" {
		return false, nil
	}
	_, err := clients.KubeClient.CoreV1().Pods(namespace).Get(name, v1.GetOptions{})

	if err != nil && !k8serrors.IsNotFound(err) {
		return false, err
	} else if err != nil && k8serrors.IsNotFound(err) {
		return false, nil
	}
	return true, nil
}

//GetPodAndNodeName will select the pod & node name for the chaos
// if the targetpod is not available it will derive the pod name randomly from the specified labels
func GetPodAndNodeName(namespace, targetPod, appLabels string, clients clients.ClientSets) (string, string, error) {
	var podName, nodeName string
	podList, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: appLabels})
	if err != nil || len(podList.Items) == 0 {
		return "", "", errors.Wrapf(err, "Fail to get the application pod in %v namespace", namespace)
	}

	isPodAvailable, err := CheckForAvailibiltyOfPod(namespace, targetPod, clients)
	if err != nil {
		return "", "", err
	}

	// getting the node name, if the target pod is defined
	// else select a random target pod from the specified labels
	if isPodAvailable {
		pod, err := clients.KubeClient.CoreV1().Pods(namespace).Get(targetPod, v1.GetOptions{})
		if err != nil {
			return "", "", err
		}
		podName = targetPod
		nodeName = pod.Spec.NodeName
	} else {
		rand.Seed(time.Now().Unix())
		randomIndex := rand.Intn(len(podList.Items))
		podName = podList.Items[randomIndex].Name
		nodeName = podList.Items[randomIndex].Spec.NodeName
	}

	return podName, nodeName, nil
}
