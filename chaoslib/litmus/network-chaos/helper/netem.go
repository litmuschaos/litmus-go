package helper

import (
	"bytes"
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
	err                                                       error
	inject, abort                                             chan os.Signal
	destIps, sPorts, dPorts, whitelistDPorts, whitelistSPorts []string
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

// Retrieve ID of the network namespace of the target container
func GetNetNS(pid int) (string, error) {

	cmd := exec.Command("sudo", "ip", "netns", "identify", fmt.Sprintf("%d", pid))

	var out, stdErr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stdErr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	netns := out.String()
	// Removing trailing newline character
	netnstrim := netns[:len(netns)-1]
	log.Infof("[Info]: Process PID=%d has netns ID=%s", pid, netnstrim)

	return netnstrim, nil
}

//preparePodNetworkChaos contains the prepration steps before chaos injection
func preparePodNetworkChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	containerID, err := common.GetRuntimeBasedContainerID(experimentsDetails.ContainerRuntime, experimentsDetails.SocketPath, experimentsDetails.TargetPods, experimentsDetails.AppNS, experimentsDetails.TargetContainer, clients)
	if err != nil {
		return err
	}
	// extract out the pid of the target container
	targetPID, err := common.GetPauseAndSandboxPID(experimentsDetails.ContainerRuntime, containerID, experimentsDetails.SocketPath)
	if err != nil {
		return err
	}

	targetNetNS, err := GetNetNS(targetPID)
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
	go abortWatcher(targetNetNS, experimentsDetails.NetworkInterface, resultDetails.Name, chaosDetails.ChaosNamespace, experimentsDetails.TargetPods)

	// injecting network chaos inside target container
	if err = injectChaos(experimentsDetails, targetNetNS); err != nil {
		return err
	}

	if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", experimentsDetails.TargetPods); err != nil {
		return err
	}

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	common.WaitForDuration(experimentsDetails.ChaosDuration)

	log.Info("[Chaos]: Stopping the experiment")

	// cleaning the netem process after chaos injection
	if err = killnetem(targetNetNS, experimentsDetails.NetworkInterface); err != nil {
		return err
	}

	return result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", experimentsDetails.TargetPods)
}

// injectChaos inject the network chaos in target container
// it is using ip netns command to enter into network namespace of target container
// and execute the netem command inside it.
func injectChaos(experimentDetails *experimentTypes.ExperimentDetails, netNS string) error {

	netemCommands := os.Getenv("NETEM_COMMAND")

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(1)
	default:
		if len(destIps) == 0 && len(sPorts) == 0 && len(dPorts) == 0 && len(whitelistDPorts) == 0 && len(whitelistSPorts) == 0 {
			tc := fmt.Sprintf("sudo ip netns exec %s tc qdisc replace dev %s root netem %v", netNS, experimentDetails.NetworkInterface, netemCommands)
			cmd := exec.Command("/bin/bash", "-c", tc)
			out, err := cmd.CombinedOutput()
			log.Info(cmd.String())
			if err != nil {
				log.Error(string(out))
				return err
			}
		} else {

			// Create a priority-based queue
			// This instantly creates classes 1:1, 1:2, 1:3
			priority := fmt.Sprintf("sudo ip netns exec %s tc qdisc replace dev %v root handle 1: prio", netNS, experimentDetails.NetworkInterface)
			cmd := exec.Command("/bin/bash", "-c", priority)
			out, err := cmd.CombinedOutput()
			log.Info(cmd.String())
			if err != nil {
				log.Error(string(out))
				return err
			}

			// Add queueing discipline for 1:3 class.
			// No traffic is going through 1:3 yet
			traffic := fmt.Sprintf("sudo ip netns exec %s tc qdisc replace dev %v parent 1:3 netem %v", netNS, experimentDetails.NetworkInterface, netemCommands)
			cmd = exec.Command("/bin/bash", "-c", traffic)
			out, err = cmd.CombinedOutput()
			log.Info(cmd.String())
			if err != nil {
				log.Error(string(out))
				return err
			}

			if len(whitelistDPorts) != 0 || len(whitelistSPorts) != 0 {
				for _, port := range whitelistDPorts {
					//redirect traffic to specific dport through band 2
					tc := fmt.Sprintf("sudo ip netns exec %s tc filter add dev %v protocol ip parent 1:0 prio 2 u32 match ip dport %v 0xffff flowid 1:2", netNS, experimentDetails.NetworkInterface, port)
					cmd = exec.Command("/bin/bash", "-c", tc)
					out, err = cmd.CombinedOutput()
					log.Info(cmd.String())
					if err != nil {
						log.Error(string(out))
						return err
					}
				}

				for _, port := range whitelistSPorts {
					//redirect traffic to specific sport through band 2
					tc := fmt.Sprintf("sudo ip netns exec %s tc filter add dev %v protocol ip parent 1:0 prio 2 u32 match ip sport %v 0xffff flowid 1:2", netNS, experimentDetails.NetworkInterface, port)
					cmd = exec.Command("/bin/bash", "-c", tc)
					out, err = cmd.CombinedOutput()
					log.Info(cmd.String())
					if err != nil {
						log.Error(string(out))
						return err
					}
				}

				tc := fmt.Sprintf("sudo ip netns exec %s tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip dst 0.0.0.0/0 flowid 1:3", netNS, experimentDetails.NetworkInterface)
				cmd = exec.Command("/bin/bash", "-c", tc)
				out, err = cmd.CombinedOutput()
				log.Info(cmd.String())
				if err != nil {
					log.Error(string(out))
					return err
				}
			} else {

				for _, ip := range destIps {
					// redirect traffic to specific IP through band 3
					tc := fmt.Sprintf("sudo ip netns exec %s tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip dst %v flowid 1:3", netNS, experimentDetails.NetworkInterface, ip)
					if strings.Contains(ip, ":") {
						tc = fmt.Sprintf("sudo ip netns exec %s tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip6 dst %v flowid 1:3", netNS, experimentDetails.NetworkInterface, ip)
					}
					cmd = exec.Command("/bin/bash", "-c", tc)
					out, err = cmd.CombinedOutput()
					log.Info(cmd.String())
					if err != nil {
						log.Error(string(out))
						return err
					}
				}

				for _, port := range sPorts {
					//redirect traffic to specific sport through band 3
					tc := fmt.Sprintf("sudo ip netns exec %s tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip sport %v 0xffff flowid 1:3", netNS, experimentDetails.NetworkInterface, port)
					cmd = exec.Command("/bin/bash", "-c", tc)
					out, err = cmd.CombinedOutput()
					log.Info(cmd.String())
					if err != nil {
						log.Error(string(out))
						return err
					}
				}

				for _, port := range dPorts {
					//redirect traffic to specific dport through band 3
					tc := fmt.Sprintf("sudo ip netns exec %s tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip dport %v 0xffff flowid 1:3", netNS, experimentDetails.NetworkInterface, port)
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
func killnetem(netNS string, networkInterface string) error {

	tc := fmt.Sprintf("sudo ip netns exec %s tc qdisc delete dev %s root", netNS, networkInterface)
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
	experimentDetails.DestinationIPs = types.Getenv("DESTINATION_IPS", "")
	experimentDetails.SourcePorts = types.Getenv("SOURCE_PORTS", "")
	experimentDetails.DestinationPorts = types.Getenv("DESTINATION_PORTS", "")

	destIps = getDestinationIPs(experimentDetails.DestinationIPs)
	if strings.TrimSpace(experimentDetails.DestinationPorts) != "" {
		if strings.Contains(experimentDetails.DestinationPorts, "!") {
			whitelistDPorts = strings.Split(strings.TrimPrefix(strings.TrimSpace(experimentDetails.DestinationPorts), "!"), ",")
		} else {
			dPorts = strings.Split(strings.TrimSpace(experimentDetails.DestinationPorts), ",")
		}
	}
	if strings.TrimSpace(experimentDetails.SourcePorts) != "" {
		if strings.Contains(experimentDetails.SourcePorts, "!") {
			whitelistSPorts = strings.Split(strings.TrimPrefix(strings.TrimSpace(experimentDetails.SourcePorts), "!"), ",")
		} else {
			sPorts = strings.Split(strings.TrimSpace(experimentDetails.SourcePorts), ",")
		}
	}
}

func getDestinationIPs(ips string) []string {
	if strings.TrimSpace(ips) == "" {
		return nil
	}
	destIPs := strings.Split(strings.TrimSpace(ips), ",")
	var uniqueIps []string

	// removing duplicates ips from the list, if any
	for i := range destIPs {
		if !common.Contains(destIPs[i], uniqueIps) {
			uniqueIps = append(uniqueIps, destIPs[i])
		}
	}
	return uniqueIps
}

// abortWatcher continuously watch for the abort signals
func abortWatcher(netNS string, networkInterface, resultName, chaosNS, targetPodName string) {

	<-abort
	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		if err = killnetem(netNS, networkInterface); err != nil {
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
