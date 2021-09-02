package helper

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

const (
	qdiscNotFound    = "Cannot delete qdisc with handle of zero"
	qdiscNoFileFound = "RTNETLINK answers: No such file or directory"
)

var (
	err           error
	inject, abort chan os.Signal
)

// Helper injects the network chaos
func Helper(clients clients.ClientSets) {

	experimentsDetails := experimentTypes.ExperimentDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}
	resultDetails := types.ResultDetails{}

	// inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	//Fetching all the ENV passed for the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	getENV(&experimentsDetails)

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	err := preparePodNetworkChaos(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails)
	if err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}

}

//preparePodNetworkChaos contains the prepration steps before chaos injection
func preparePodNetworkChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	containerID, err := getContainerID(experimentsDetails, clients)
	if err != nil {
		return err
	}
	// extract out the pid of the target container
	targetPID, err := common.GetPID(experimentsDetails.ContainerRuntime, containerID, experimentsDetails.SocketPath)
	if err != nil {
		return err
	}

	// record the event inside chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// watching for the abort signal and revert the chaos
	go abortWatcher(targetPID, resultDetails.Name, chaosDetails.ChaosNamespace, experimentsDetails.TargetPods)

	// injecting network chaos inside target container
	if err = injectChaos(experimentsDetails, targetPID); err != nil {
		return err
	}

	if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", experimentsDetails.TargetPods); err != nil {
		return err
	}

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	common.WaitForDuration(experimentsDetails.ChaosDuration)

	log.Info("[Chaos]: Stopping the experiment")

	// cleaning the netem process after chaos injection
	if err = killnetem(targetPID); err != nil {
		return err
	}

	if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", experimentsDetails.TargetPods); err != nil {
		return err
	}

	return nil
}

//getContainerID extract out the container id of the target container
func getContainerID(experimentDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {

	var containerID string
	switch experimentDetails.ContainerRuntime {
	case "docker":
		host := "unix://" + experimentDetails.SocketPath
		// deriving the container id of the pause container
		cmd := "sudo docker --host " + host + " ps | grep k8s_POD_" + experimentDetails.TargetPods + "_" + experimentDetails.AppNS + " | awk '{print $1}'"
		out, err := exec.Command("/bin/sh", "-c", cmd).CombinedOutput()
		if err != nil {
			log.Error(fmt.Sprintf("[docker]: Failed to run docker ps command: %s", string(out)))
			return "", err
		}
		containerID = strings.TrimSpace(string(out))
	case "containerd", "crio":
		containerID, err = common.GetContainerID(experimentDetails.AppNS, experimentDetails.TargetPods, experimentDetails.TargetContainer, clients)
		if err != nil {
			return containerID, err
		}
	default:
		return "", errors.Errorf("%v container runtime not suported", experimentDetails.ContainerRuntime)
	}
	log.Infof("Container ID: %v", containerID)

	return containerID, nil
}

// injectChaos inject the network chaos in target container
// it is using nsenter command to enter into network namespace of target container
// and execute the netem command inside it.
func injectChaos(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {

	netemCommands := os.Getenv("NETEM_COMMAND")
	destinationIPs := os.Getenv("DESTINATION_IPS")

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(1)
	default:
		if destinationIPs == "" {
			tc := fmt.Sprintf("sudo nsenter -t %d -n tc qdisc replace dev %s root netem %v", pid, experimentDetails.NetworkInterface, netemCommands)
			cmd := exec.Command("/bin/bash", "-c", tc)
			out, err := cmd.CombinedOutput()
			log.Info(cmd.String())
			if err != nil {
				log.Error(string(out))
				return err
			}
		} else {

			ips := strings.Split(destinationIPs, ",")
			var uniqueIps []string

			// removing duplicates ips from the list, if any
			for i := range ips {
				isPresent := false
				for j := range uniqueIps {
					if ips[i] == uniqueIps[j] {
						isPresent = true
					}
				}
				if !isPresent {
					uniqueIps = append(uniqueIps, ips[i])
				}

			}

			// Create a priority-based queue
			// This instantly creates classes 1:1, 1:2, 1:3
			priority := fmt.Sprintf("sudo nsenter -t %v -n tc qdisc replace dev %v root handle 1: prio", pid, experimentDetails.NetworkInterface)
			cmd := exec.Command("/bin/bash", "-c", priority)
			out, err := cmd.CombinedOutput()
			log.Info(cmd.String())
			if err != nil {
				log.Error(string(out))
				return err
			}

			// Add queueing discipline for 1:3 class.
			// No traffic is going through 1:3 yet
			traffic := fmt.Sprintf("sudo nsenter -t %v -n tc qdisc replace dev %v parent 1:3 netem %v", pid, experimentDetails.NetworkInterface, netemCommands)
			cmd = exec.Command("/bin/bash", "-c", traffic)
			out, err = cmd.CombinedOutput()
			log.Info(cmd.String())
			if err != nil {
				log.Error(string(out))
				return err
			}

			for _, ip := range uniqueIps {

				// redirect traffic to specific IP through band 3
				// It allows ipv4 addresses only
				if !strings.Contains(ip, ":") {
					tc := fmt.Sprintf("sudo nsenter -t %v -n tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip dst %v flowid 1:3", pid, experimentDetails.NetworkInterface, ip)
					cmd = exec.Command("/bin/bash", "-c", tc)
					out, err = cmd.CombinedOutput()
					log.Info(cmd.String())
					if err != nil {
						log.Error(string(out))
						return err
					}
				}
			}
		}
	}
	return nil
}

// killnetem kill the netem process for all the target containers
func killnetem(PID int) error {

	tc := fmt.Sprintf("sudo nsenter -t %d -n tc qdisc delete dev eth0 root", PID)
	cmd := exec.Command("/bin/bash", "-c", tc)
	out, err := cmd.CombinedOutput()
	log.Info(cmd.String())

	if err != nil {
		log.Error(string(out))
		// ignoring err if qdisc process doesn't exist inside the target container
		if strings.Contains(string(out), qdiscNotFound) || strings.Contains(string(out), qdiscNoFileFound) {
			log.Warn("The network chaos process has already been removed")
			return nil
		}
		return err
	}

	return nil
}

//getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = common.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = common.Getenv("INSTANCE_ID", "")
	experimentDetails.AppNS = common.Getenv("APP_NS", "")
	experimentDetails.TargetContainer = common.Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPods = common.Getenv("APP_POD", "")
	experimentDetails.AppLabel = common.Getenv("APP_LABEL", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(common.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = common.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = common.Getenv("CHAOS_ENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(common.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = common.Getenv("POD_NAME", "")
	experimentDetails.ContainerRuntime = common.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.NetworkInterface = common.Getenv("NETWORK_INTERFACE", "eth0")
	experimentDetails.SocketPath = common.Getenv("SOCKET_PATH", "")
	experimentDetails.DestinationIPs = common.Getenv("DESTINATION_IPS", "")
}

// abortWatcher continuosly watch for the abort signals
func abortWatcher(targetPID int, resultName, chaosNS, targetPodName string) {

	<-abort
	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		if err = killnetem(targetPID); err != nil {
			log.Errorf("unable to kill netem process, err :%v", err)
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	if err = result.AnnotateChaosResult(resultName, chaosNS, "reverted", "pod", targetPodName); err != nil {
		log.Errorf("unable to annotate the chaosresult, err :%v", err)
	}
	log.Info("Chaos Revert Completed")
	os.Exit(1)
}
