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
	if err = revertChaos(experimentsDetails, targetPID); err != nil {
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

// injectChaos inject the http chaos in target container and add ruleset to the iptables to redirect the ports
func injectChaos(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(1)
	default:
		// proceed for chaos injection
		if err := startProxy(experimentDetails, pid); err != nil {
			_ = killProxy(experimentDetails, pid)
			return errors.Errorf("failed to start proxy, err: %v", err)
		}
		if err := addIPRuleSet(experimentDetails, pid); err != nil {
			return errors.Errorf("failed to add ip rule set, err: %v", err)
		}
	}
	return nil
}

// revertChaos revert the http chaos in target container
func revertChaos(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {

	if err := removeIPRuleSet(experimentDetails, pid); err != nil {
		return errors.Errorf("failed to remove ip rule set, err: %v", err)
	}

	if err := killProxy(experimentDetails, pid); err != nil {
		return errors.Errorf("failed to kill proxy, err: %v", err)
	}

	return nil
}

// startProxy starts the proxy process inside the target container
// it is using nsenter command to enter into network namespace of target container
// and execute the proxy related command inside it.
func startProxy(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {

	toxicCommands := os.Getenv("TOXIC_COMMAND")

	startProxyServerCommand := fmt.Sprintf("(sudo nsenter -t %d -n /litmus/toxiproxy -host=0.0.0.0 > /dev/null 2>&1 &)", pid)
	createProxyCommand := fmt.Sprintf("(sudo nsenter -t %d -n /litmus/toxiproxy-cli create -l 0.0.0.0:%d -u %v:%d proxy)", pid, experimentDetails.ListenPort, experimentDetails.TargetHost, experimentDetails.TargetPort)
	createToxicCommand := ""
	switch experimentDetails.ExperimentName {
	// preparing command for toxic addition based on HttpChaosType chosen by user
	case "pod-http-latency":
		createToxicCommand = fmt.Sprintf("(sudo nsenter -t %d -n /litmus/toxiproxy-cli toxic add %s proxy)", pid, toxicCommands)
	}

	// sleep 10 is added for proxy-server to be ready for creating proxy and adding toxics
	chaosCommand := fmt.Sprintf("%s && sleep 10 && %s && %s", startProxyServerCommand, createProxyCommand, createToxicCommand)

	cmd := exec.Command("/bin/bash", "-c", chaosCommand)
	log.Infof("[Chaos]: Starting proxy server: %s", cmd.String())

	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	log.Info("Proxy started successfully")
	return nil
}

// killProxy kills the proxy process inside the target container
// it is using nsenter command to enter into network namespace of target container
// and execute the proxy related command inside it.
func killProxy(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {
	stopProxyServerCommand := fmt.Sprintf("sudo nsenter -t %d -n sudo kill -9 $(ps aux | grep [t]oxiproxy | awk 'FNR==1{print $1}')", pid)
	cmd := exec.Command("/bin/bash", "-c", stopProxyServerCommand)
	log.Infof("[Chaos]: Stopping proxy server: %s", cmd.String())

	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	log.Info("Proxy stopped successfully")
	return nil
}

// addIPRuleSet adds the ip rule set to iptables in target container
// it is using nsenter command to enter into network namespace of target container
// and execute the iptables related command inside it.
func addIPRuleSet(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {
	addIPRuleSetCommand := fmt.Sprintf("(sudo nsenter -t %d -n iptables -t nat -A PREROUTING -i eth0 -p tcp --dport %d -j REDIRECT --to-port %d)", pid, experimentDetails.TargetPort, experimentDetails.ListenPort)
	cmd := exec.Command("/bin/bash", "-c", addIPRuleSetCommand)
	log.Infof("[Chaos]: Adding IPtables ruleset: %s", cmd.String())

	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	log.Info("IP rule set added successfully")
	return nil
}

// removeIPRuleSet removes the ip rule set from iptables in target container
// it is using nsenter command to enter into network namespace of target container
// and execute the iptables related command inside it.
func removeIPRuleSet(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {
	removeIPRuleSetCommand := fmt.Sprintf("sudo nsenter -t %d -n iptables -t nat -D PREROUTING -i eth0 -p tcp --dport %d -j REDIRECT --to-port %d", pid, experimentDetails.TargetPort, experimentDetails.ListenPort)
	cmd := exec.Command("/bin/bash", "-c", removeIPRuleSetCommand)
	log.Infof("[Chaos]: Removing IPtables ruleset: %s", cmd.String())

	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	log.Info("IP rule set removed successfully")
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
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "")
	experimentDetails.TargetPort, _ = strconv.Atoi(types.Getenv("TARGET_PORT", ""))
	experimentDetails.ListenPort, _ = strconv.Atoi(types.Getenv("LISTEN_PORT", ""))
}

// abortWatcher continuously watch for the abort signals
func abortWatcher(targetPID int, resultName, chaosNS string, experimentDetails *experimentTypes.ExperimentDetails) {

	<-abort
	log.Info("[Abort]: Killing process started because of terminated signal received")
	log.Info("[Abort]: Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		if err = revertChaos(experimentDetails, targetPID); err != nil {
			retry--
			if retry > 0 {
				log.Errorf("[Abort]: Failed to remove Proxy and IPtables ruleset, retrying %d more times, err: %v", retry, err)
			} else {
				log.Errorf("[Abort]: Failed to remove Proxy and IPtables ruleset, err: %v", err)
			}
			time.Sleep(1 * time.Second)
			continue
		}
		log.Infof("[Abort]: Proxy and IPtables ruleset removed successfully")
		break
	}

	if retry > 0 {
		if err = result.AnnotateChaosResult(resultName, chaosNS, "reverted", "pod", experimentDetails.TargetPods); err != nil {
			log.Errorf("unable to annotate the chaosresult, err :%v", err)
		}
		log.Info("[Abort]: Chaos Revert Completed")
		os.Exit(1)
	}
	log.Errorf("[Abort]: Chaos Revert Failed")
	os.Exit(1)
}