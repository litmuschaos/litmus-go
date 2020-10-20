package common

import (
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
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

//DeleteAllPod deletes all the pods with matching labels and wait until all the pods got terminated
func DeleteAllPod(podLabel, namespace string, timeout, delay int, clients clients.ClientSets) error {

	err := clients.KubeClient.CoreV1().Pods(namespace).DeleteCollection(&v1.DeleteOptions{}, v1.ListOptions{LabelSelector: podLabel})

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
func GetPodList(namespace, targetPod, appLabels, uid string, podAffPerc int, clients clients.ClientSets) (core_v1.PodList, error) {
	realpods := core_v1.PodList{}
	nonChaosPods := core_v1.PodList{}
	podList, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: appLabels})
	if err != nil || len(podList.Items) == 0 {
		return core_v1.PodList{}, errors.Wrapf(err, "Failed to find the pod with matching labels in %v namespace", namespace)
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

		if appLabels == "" {
			// ignore chaos pods
			for index, pod := range podList.Items {
				if !(pod.Labels["chaosUID"] == uid || pod.Labels["name"] == "chaos-operator") {
					nonChaosPods.Items = append(nonChaosPods.Items, podList.Items[index])
				}

			}

		} else {
			nonChaosPods.Items = podList.Items
		}

		newPodListLength := math.Maximum(1, math.Adjustment(podAffPerc, len(nonChaosPods.Items)))
		rand.Seed(time.Now().UnixNano())

		// it will generate the random podlist
		// it starts from the random index and choose requirement no of pods next to that index in a circular way.
		index := rand.Intn(len(nonChaosPods.Items))
		for i := 0; i < newPodListLength; i++ {
			realpods.Items = append(realpods.Items, nonChaosPods.Items[index])
			index = (index + 1) % len(nonChaosPods.Items)
		}

		log.Infof("[Chaos]:Number of pods targeted: %v", strconv.Itoa(newPodListLength))
	}

	return realpods, nil
}

// GetChaosPodAnnotation will return the annotation on chaos pod
func GetChaosPodAnnotation(podName, namespace string, clients clients.ClientSets) (map[string]string, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(namespace).Get(podName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pod.Annotations, nil
}

//GetNodeName will select a random replica of application pod and return the node name of that application pod
func GetNodeName(namespace, labels string, clients clients.ClientSets) (string, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: labels})
	if err != nil || len(podList.Items) == 0 {
		return "", errors.Wrapf(err, "Failed to find the application pods with matching labels in %v namespace, err: %v", namespace, err)
	}

	rand.Seed(time.Now().Unix())
	randomIndex := rand.Intn(len(podList.Items))
	nodeName := podList.Items[randomIndex].Spec.NodeName

	return nodeName, nil
}

//GetNodeList will select a random replica of application pod and return the node name of that application pod
func GetNodeList(namespace, labels string, nodeAffPerc int, nodeName string, clients clients.ClientSets) ([]string, error) {

	var nodeList []string

	if nodeName != "" {
		nodeList = append(nodeList, nodeName)
		return nodeList, nil
	}

	if labels != "" {
		podList, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: labels})
		if err != nil || len(podList.Items) == 0 {
			return nil, errors.Wrapf(err, "Failed to find the application pods with matching labels in %v namespace, err: %v", namespace, err)
		}

		rand.Seed(time.Now().Unix())
		randomIndex := rand.Intn(len(podList.Items))
		nodeName := podList.Items[randomIndex].Spec.NodeName
		nodeList = append(nodeList, nodeName)
	} else {
		nodes, err := clients.KubeClient.CoreV1().Nodes().List(v1.ListOptions{})
		if err != nil || len(nodes.Items) == 0 {
			return nil, errors.Wrapf(err, "Failed to find the nodes, err: %v", namespace, err)
		}

		newNodeListLength := math.Maximum(1, math.Adjustment(nodeAffPerc, len(nodes.Items)))
		rand.Seed(time.Now().UnixNano())

		// it will generate the random nodelist
		// it starts from the random index and choose requirement no of pods next to that index in a circular way.
		index := rand.Intn(len(nodes.Items))
		for i := 0; i < newNodeListLength; i++ {
			nodeList = append(nodeList, nodes.Items[index].Name)
			index = (index + 1) % len(nodes.Items)
		}

	}

	return nodeList, nil
}

// AbortWatcher continuosly watch for the abort signals
// it will update chaosresult w/ failed step and create an abort event, if it recieved abort signal during chaos
func AbortWatcher(expname string, clients clients.ClientSets, resultDetails *types.ResultDetails, chaosDetails *types.ChaosDetails, eventsDetails *types.EventDetails) {

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)

	for {
		select {
		case <-signChan:
			log.Info("[Chaos]: Chaos Experiment Abortion started because of terminated signal received")
			// updating the chaosresult after stopped
			failStep := "Chaos injection stopped!"
			types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
			result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

			// generating summary event in chaosengine
			msg := expname + " experiment has been aborted"
			types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

			// generating summary event in chaosresult
			types.SetResultEventAttributes(eventsDetails, types.StoppedVerdict, msg, "Warning", resultDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")
			os.Exit(1)
		}
	}
}
