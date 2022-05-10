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
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

var (
	err           error
	inject, abort chan os.Signal
)

// Helper injects the http chaos
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
	types.InitialiseChaosVariables(&chaosDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	err := prepareK8sHttpChaos(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails)
	if err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}

}

//prepareK8sHttpChaos contains the prepration steps before chaos injection
func prepareK8sHttpChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

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
	go abortWatcher(targetPID, resultDetails.Name, chaosDetails.ChaosNamespace, experimentsDetails)

	// injecting http chaos inside target container
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
	if err = killProxy(targetPID, experimentsDetails); err != nil {
		return err
	}

	return result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", experimentsDetails.TargetPods)
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

// injectChaos inject the http chaos in target container
// it is using nsenter command to enter into network namespace of target container
// and execute the proxy related command inside it.
func injectChaos(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(1)
	default:
		addIPRuleSetCommand := fmt.Sprintf("(sudo nsenter -t %d -n iptables -t nat -A PREROUTING -i eth0 -p tcp --dport %d -j REDIRECT --to-port %d)", pid, experimentDetails.TargetPort, experimentDetails.DestinationPort)
		startProxyServerCommand := fmt.Sprintf("(sudo nsenter -t %d -n /litmus/toxiproxy -host=0.0.0.0 > /dev/null 2>&1 &)", pid)
		createProxyCommand := fmt.Sprintf("(sudo nsenter -t %d -n /litmus/toxiproxy-cli create -l 0.0.0.0:%d -u localhost:%d proxy)", pid, experimentDetails.DestinationPort, experimentDetails.TargetPort)
		createToxicCommand := ""
		switch experimentDetails.HttpChaosType {
		// preparing command for toxic addition based on HttpChaosType chosen by user
		case "http-latency":
			createToxicCommand = fmt.Sprintf("(sudo nsenter -t %d -n /litmus/toxiproxy-cli toxic add -t latency -a latency=%d proxy)", pid, experimentDetails.HttpLatency)
		}

		// sleep 10 is added for proxy-server to be ready for creating proxy and adding toxics
		chaosCommand := fmt.Sprintf("%s && sleep 10 && %s && %s && %s", startProxyServerCommand, createProxyCommand, createToxicCommand, addIPRuleSetCommand)
		cmd := exec.Command("/bin/bash", "-c", chaosCommand)
		out, err := cmd.CombinedOutput()
		log.Info(cmd.String())
		if err != nil {
			log.Error(string(out))
			return err
		}
	}
	return nil
}

// killnetem kill the netem process for all the target containers
func killProxy(PID int, experimentDetails *experimentTypes.ExperimentDetails) error {

	removeIPRuleSetCommand := fmt.Sprintf("sudo nsenter -t %d -n iptables -t nat -D PREROUTING -i eth0 -p tcp --dport %d -j REDIRECT --to-port %d", PID, experimentDetails.TargetPort, experimentDetails.DestinationPort)
	stopProxyServerCommand := fmt.Sprintf("sudo nsenter -t %d -n sudo kill -9 $(ps aux | grep [t]oxiproxy | awk 'FNR==1{print $1}')", PID)
	deleteProxyCommand := fmt.Sprintf("sudo nsenter -t %d -n /litmus/toxiproxy-cli delete proxy", PID)
	killCommand := fmt.Sprintf("%s && %s && %s", removeIPRuleSetCommand, deleteProxyCommand, stopProxyServerCommand)
	cmd := exec.Command("/bin/bash", "-c", killCommand)
	out, err := cmd.CombinedOutput()
	log.Info(cmd.String())

	if err != nil {
		log.Error(string(out))
		// ignoring err if qdisc process doesn't exist inside the target container
		// if strings.Contains(string(out), qdiscNotFound) || strings.Contains(string(out), qdiscNoFileFound) {
		// 	log.Warn("The http chaos process has already been removed")
		// 	return nil
		// }
		return err
	}

	return nil
}

//getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.AppNS = types.Getenv("APP_NAMESPACE", "")
	experimentDetails.TargetContainer = types.Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPods = types.Getenv("APP_POD", "")
	experimentDetails.AppLabel = types.Getenv("APP_LABEL", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.NetworkInterface = types.Getenv("NETWORK_INTERFACE", "eth0")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "")
	experimentDetails.TargetPort, _ = strconv.Atoi(types.Getenv("TARGET_PORT", "8080"))
	experimentDetails.DestinationPort, _ = strconv.Atoi(types.Getenv("DESTINATION_PORT", "9091"))
	experimentDetails.HttpLatency, _ = strconv.Atoi(types.Getenv("HTTP_LATENCY", "0"))
	experimentDetails.HttpChaosType = types.Getenv("HTTP_CHAOS_TYPE", "http-latency")
}

// abortWatcher continuously watch for the abort signals
func abortWatcher(targetPID int, resultName, chaosNS string, experimentDetails *experimentTypes.ExperimentDetails) {

	<-abort
	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		if err = killProxy(targetPID, experimentDetails); err != nil {
			log.Errorf("unable to kill proxy process, err :%v", err)
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	if err = result.AnnotateChaosResult(resultName, chaosNS, "reverted", "pod", experimentDetails.TargetPods); err != nil {
		log.Errorf("unable to annotate the chaosresult, err :%v", err)
	}
	log.Info("Chaos Revert Completed")
	os.Exit(1)
}
