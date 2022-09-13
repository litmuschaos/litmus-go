package helper

import (
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
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
	types.InitialiseChaosVariables(&chaosDetails)

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

	targetEnv := os.Getenv("TARGETS")
	if targetEnv == "" {
		return fmt.Errorf("no target found, provide atleast one target")
	}

	var targets []targetDetails

	for _, t := range strings.Split(targetEnv, ";") {
		target := strings.Split(t, "&")
		if len(target) != 3 {
			return fmt.Errorf("unsupported target: '%v', provide target in '<name>&<namespace>&<destinationIps>", target)
		}
		td := targetDetails{
			Name:            target[0],
			Namespace:       target[1],
			DestinationIps:  target[2],
			TargetContainer: experimentsDetails.TargetContainer,
		}

		if td.TargetContainer == "" {
			td.TargetContainer, err = common.GetTargetContainer(td.Namespace, td.Name, clients)
			if err != nil {
				return errors.Errorf("unable to get the target container name, err: %v", err)
			}
		}
		td.ContainerId, err = common.GetRuntimeBasedContainerID(experimentsDetails.ContainerRuntime, experimentsDetails.SocketPath, td.Name, td.Namespace, td.TargetContainer, clients)
		if err != nil {
			return err
		}

		// extract out the pid of the target container
		td.Pid, err = common.GetPID(experimentsDetails.ContainerRuntime, td.ContainerId, experimentsDetails.SocketPath)
		if err != nil {
			return err
		}
		targets = append(targets, td)
	}

	// watching for the abort signal and revert the chaos
	go abortWatcher(targets, experimentsDetails.NetworkInterface, resultDetails.Name, chaosDetails.ChaosNamespace)

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(1)
	default:
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	for _, t := range targets {
		// injecting network chaos inside target container
		if err = injectChaos(experimentsDetails.NetworkInterface, t); err != nil {
			return err
		}
		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", t.Name); err != nil {
			_, err := killnetem(t, experimentsDetails.NetworkInterface)
			return err
		}
	}

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	common.WaitForDuration(experimentsDetails.ChaosDuration)

	log.Info("[Chaos]: Stopping the experiment")

	var errList []string
	for _, t := range targets {
		// cleaning the netem process after chaos injection
		killed, err := killnetem(t, experimentsDetails.NetworkInterface)
		if !killed && err != nil {
			errList = append(errList, err.Error())
			continue
		}
		if killed && err == nil {
			if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", t.Name); err != nil {
				errList = append(errList, err.Error())
			}
		}
	}

	if len(errList) != 0 {
		return fmt.Errorf("err: %v", strings.Join(errList, ","))
	}

	return nil
}

// injectChaos inject the network chaos in target container
// it is using nsenter command to enter into network namespace of target container
// and execute the netem command inside it.
func injectChaos(netInterface string, target targetDetails) error {

	netemCommands := os.Getenv("NETEM_COMMAND")

	if target.DestinationIps == "" {
		tc := fmt.Sprintf("sudo nsenter -t %d -n tc qdisc replace dev %s root netem %v", target.Pid, netInterface, netemCommands)
		cmd := exec.Command("/bin/bash", "-c", tc)
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Info(cmd.String())
			log.Error(string(out))
			return err
		}
	} else {

		ips := strings.Split(target.DestinationIps, ",")
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
		priority := fmt.Sprintf("sudo nsenter -t %v -n tc qdisc replace dev %v root handle 1: prio", target.Pid, netInterface)
		cmd := exec.Command("/bin/bash", "-c", priority)
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Info(cmd.String())
			log.Error(string(out))
			return err
		}

		// Add queueing discipline for 1:3 class.
		// No traffic is going through 1:3 yet
		traffic := fmt.Sprintf("sudo nsenter -t %v -n tc qdisc replace dev %v parent 1:3 netem %v", target.Pid, netInterface, netemCommands)
		cmd = exec.Command("/bin/bash", "-c", traffic)
		out, err = cmd.CombinedOutput()
		if err != nil {
			log.Info(cmd.String())
			log.Error(string(out))
			return err
		}

		for _, ip := range uniqueIps {

			// redirect traffic to specific IP through band 3
			tc := fmt.Sprintf("sudo nsenter -t %v -n tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip dst %v flowid 1:3", target.Pid, netInterface, ip)
			if strings.Contains(ip, ":") {
				tc = fmt.Sprintf("sudo nsenter -t %v -n tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip6 dst %v flowid 1:3", target.Pid, netInterface, ip)
			}
			cmd = exec.Command("/bin/bash", "-c", tc)
			out, err = cmd.CombinedOutput()
			if err != nil {
				log.Info(cmd.String())
				log.Error(string(out))
				return err
			}
		}
	}
	log.Infof("chaos injected successfully on {pod: %v, container: %v}", target.Name, target.TargetContainer)
	return nil
}

// killnetem kill the netem process for all the target containers
func killnetem(target targetDetails, networkInterface string) (bool, error) {

	tc := fmt.Sprintf("sudo nsenter -t %d -n tc qdisc delete dev %s root", target.Pid, networkInterface)
	cmd := exec.Command("/bin/bash", "-c", tc)
	out, err := cmd.CombinedOutput()

	if err != nil {
		log.Info(cmd.String())
		// ignoring err if qdisc process doesn't exist inside the target container
		if strings.Contains(string(out), qdiscNotFound) || strings.Contains(string(out), qdiscNoFileFound) {
			log.Warn("The network chaos process has already been removed")
			return true, err
		}
		log.Error(string(out))
		return false, err
	}
	log.Infof("chaos reverted successfully on {pod: %v, container: %v}", target.Name, target.TargetContainer)

	return true, nil
}

type targetDetails struct {
	Name            string
	Namespace       string
	DestinationIps  string
	TargetContainer string
	ContainerId     string
	Pid             int
}

//getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.TargetContainer = types.Getenv("APP_CONTAINER", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", ""))
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.NetworkInterface = types.Getenv("NETWORK_INTERFACE", "")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "")
}

// abortWatcher continuously watch for the abort signals
func abortWatcher(targets []targetDetails, networkInterface, resultName, chaosNS string) {

	<-abort
	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		for _, t := range targets {
			killed, err := killnetem(t, networkInterface)
			if err != nil && !killed {
				log.Errorf("unable to kill netem process, err :%v", err)
				continue
			}
			if killed && err == nil {
				if err = result.AnnotateChaosResult(resultName, chaosNS, "reverted", "pod", t.Name); err != nil {
					log.Errorf("unable to annotate the chaosresult, err :%v", err)
				}
			}
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	log.Info("Chaos Revert Completed")
	os.Exit(1)
}
