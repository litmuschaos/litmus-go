package network_latency

import (
	"fmt"
	. "fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/chaoslib/litmus/network_latency/cri"
	"github.com/litmuschaos/litmus-go/chaoslib/litmus/network_latency/tc"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	env "github.com/litmuschaos/litmus-go/pkg/generic/network-latency/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-latency/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PrepareNetwork function orchestrates the experiment
func PrepareNetwork(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails) error {

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	log.Info("[Chaos Target]: Fetching dependency info")
	deps, err := env.Dependencies()
	if err != nil {
		Println(err.Error())
		return err
	}

	log.Info("[Chaos Target]: Resolving latency target IPs")
	conf, err := env.Resolver(deps)
	if err != nil {
		Println(err.Error())
		return err
	}

	log.Info("[Chaos Target]: Finding the container PID")
	targetPIDs, err := ChaosTargetPID(experimentsDetails.ChaosNode, experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		Println(err.Error())
		return err
	}

	for _, targetPID := range targetPIDs {
		log.Info(fmt.Sprintf("[Chaos]: Apply latency to process PID=%d", targetPID))
		err := tc.CreateDelayQdisc(targetPID, experimentsDetails.Latency, experimentsDetails.Jitter)
		if err != nil {
			log.Error("Failed to create delay, aborting experiment")
			return err
		}

		for i, ip := range conf.IP {
			port := conf.Port[i]
			err = tc.AddIPFilter(targetPID, ip, port)
			if err != nil {
				Println(err.Error())
				return err
			}
		}
	}

	log.Infof("[Chaos]: Waiting for %vs", strconv.Itoa(experimentsDetails.ChaosDuration))

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
			for _, targetPID := range targetPIDs {
				err = tc.Killnetem(targetPID)
				if err != nil {
					Println(err.Error())
				}
			}
			os.Exit(1)
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			endTime = nil
			break loop
		}
	}

	log.Info("[Chaos]: Stopping the experiment")
	for _, targetPID := range targetPIDs {
		err = tc.Killnetem(targetPID)
		if err != nil {
			Println(err.Error())
		}
	}
	if err != nil {
		Println(err.Error())
		return err
	}
	return nil
}

//ChaosTargetPID finds the target app PIDs
func ChaosTargetPID(chaosNode string, appNs string, appLabel string, clients clients.ClientSets) ([]int, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(appNs).List(metav1.ListOptions{
		LabelSelector: appLabel,
		FieldSelector: "spec.nodeName=" + chaosNode,
	})
	if err != nil {
		return []int{}, err
	}

	if len(podList.Items) == 0 {
		return []int{}, errors.Errorf("No pods with label %s were found in the namespace %s", appLabel, appNs)
	}

	PIDs := []int{}
	for _, pod := range podList.Items {
		if len(pod.Status.ContainerStatuses) == 0 {
			return []int{}, errors.Errorf("Unreachable: No containers running in this pod: %+v", pod)
		}

		// containers in a pod share the network namespace, so anyone should be
		// fine for our purposes
		container := pod.Status.ContainerStatuses[0]

		log.InfoWithValues("Found target container", logrus.Fields{
			"container":   container.Name,
			"Pod":         pod.Name,
			"Status":      pod.Status.Phase,
			"containerID": container.ContainerID,
		})

		PID, err := cri.PIDFromContainer(container)
		if err != nil {
			return []int{}, err
		}

		PIDs = append(PIDs, PID)
	}

	log.Info(Sprintf("Found %d target process(es)", len(PIDs)))
	return PIDs, nil
}
