package common

import (
	"math/rand"
	"strconv"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
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
	rand.Seed(time.Now().UnixNano())
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

//GetPodList check for the availibilty of the target pod for the chaos execution
// if the target pod is not defined it will derive the random target pod list using pod affected percentage
func GetPodList(namespace, targetPod, appLabels string, podAffPerc int, clients clients.ClientSets) (core_v1.PodList, error) {
	realpods := core_v1.PodList{}
	podList, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: appLabels})
	if err != nil || len(podList.Items) == 0 {
		return core_v1.PodList{}, errors.Wrapf(err, "Fail to list the application pod in %v namespace", namespace)
	}

	isPodAvailable, err := CheckForAvailibiltyOfPod(namespace, targetPod, clients)
	if err != nil {
		return core_v1.PodList{}, err
	}

	// getting the node name, if the target pod is defined
	// else select a random target pod from the specified labels
	if isPodAvailable {
		pod, err := clients.KubeClient.CoreV1().Pods(namespace).Get(targetPod, v1.GetOptions{})
		if err != nil {
			return core_v1.PodList{}, err
		}
		realpods.Items = append(realpods.Items, *pod)
	} else {
		newPodListLength := math.Maximum(1, math.Adjustment(podAffPerc, len(podList.Items)))
		realpods.Items = podList.Items[:newPodListLength]

		log.Infof("[Chaos]:Number of pods targeted: %v", strconv.Itoa(newPodListLength))
	}

	return realpods, nil
}

// DeleteHelperDaemonset deletes the specified daemonset and wait until it got terminated
func DeleteHelperDaemonset(name, labels, namespace string, timeout, delay int, clients clients.ClientSets) error {
	if err := clients.KubeClient.AppsV1().DaemonSets(namespace).Delete(name, &v1.DeleteOptions{}); err != nil {
		return err
	}

	// waiting for the termination of the daemonset
	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			dsList, err := clients.KubeClient.AppsV1().DaemonSets(namespace).List(v1.ListOptions{LabelSelector: labels})
			if err != nil || len(dsList.Items) != 0 {
				return errors.Errorf("Unable to delete the daemonset, err: %v", err)
			}
			return nil
		})

	return err

}
