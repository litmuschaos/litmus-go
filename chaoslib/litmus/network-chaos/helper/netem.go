package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/chaoslib/litmus/network_latency/tc"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

var err error

func main() {

	experimentsDetails := experimentTypes.ExperimentDetails{}
	clients := clients.ClientSets{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}
	resultDetails := types.ResultDetails{}

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Fatalf("Unable to Get the kubeconfig, err: %v", err)
	}

	//Fetching all the ENV passed for the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	GetENV(&experimentsDetails)

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	err := PreparePodNetworkChaos(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails)
	if err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}

}

//PreparePodNetworkChaos contains the prepration steps before chaos injection
func PreparePodNetworkChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	// extract out the pid of the target container
	targetPID, err := GetPID(experimentsDetails, clients)
	if err != nil {
		return err
	}

	// record the event inside chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	// injecting network chaos inside target container
	if err = InjectChaos(experimentsDetails, targetPID); err != nil {
		return err
	}

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

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
			// updating the chaosresult after stopped
			failStep := "Network Chaos injection stopped!"
			types.SetResultAfterCompletion(resultDetails, "Stopped", "Stopped", failStep)
			result.ChaosResult(chaosDetails, clients, resultDetails, "EOT")

			// generating summary event in chaosengine
			msg := experimentsDetails.ExperimentName + " experiment has been aborted"
			types.SetEngineEventAttributes(eventsDetails, types.Summary, msg, "Warning", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

			// generating summary event in chaosresult
			types.SetResultEventAttributes(eventsDetails, types.StoppedVerdict, msg, "Warning", resultDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosResult")

			if err = tc.Killnetem(targetPID); err != nil {
				log.Errorf("unable to kill netem process, err :%v", err)

			}
			os.Exit(1)
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			endTime = nil
			break loop
		}
	}

	log.Info("[Chaos]: Stopping the experiment")

	// cleaning the netem process after chaos injection
	if err = tc.Killnetem(targetPID); err != nil {
		return err
	}

	return nil
}

//GetPID extract out the pid of target container
func GetPID(experimentDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (int, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(experimentDetails.AppNS).Get(experimentDetails.TargetPods, v1.GetOptions{})
	if err != nil {
		return 0, err
	}

	var containerID string

	// filtering out the container id from the details of containers inside containerStatuses of the given pod
	// container id is present in the form of <runtime>://<container-id>
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == experimentDetails.TargetContainer {
			containerID = strings.Split(container.ContainerID, "//")[1]
			break
		}
	}

	log.Infof("containerid: %v", containerID)

	// deriving pid from the inspect out of target container
	out, err := exec.Command("crictl", "inspect", containerID).CombinedOutput()
	if err != nil {
		log.Error(fmt.Sprintf("[cri]: Failed to run crictl: %s", string(out)))
		return 0, err
	}
	// parsing data from the json output of inspect command
	PID, err := parsePIDFromJSON(out, experimentDetails.ContainerRuntime)
	if err != nil {
		log.Error(fmt.Sprintf("[cri]: Failed to parse json from crictl output: %s", string(out)))
		return 0, err
	}

	log.Info(fmt.Sprintf("[cri]: Container ID=%s has process PID=%d", containerID, PID))

	return PID, nil

}

// InspectResponse JSON representation of crictl inspect command output
// in crio, pid is present inside pid attribute of inspect output
// in containerd, pid is present inside `info.pid` of inspect output
type InspectResponse struct {
	Info InfoDetails `json:"info"`
}

// InfoDetails JSON representation of crictl inspect command output
// in crio, pid is present inside pid attribute of inspect output
// in containerd, pid is present inside `info.pid` of inspect output
type InfoDetails struct {
	PID int `json:"pid"`
}

//parsePIDFromJSON extract the pid from the json output
func parsePIDFromJSON(j []byte, runtime string) (int, error) {

	var pid int

	// in crio, pid is present inside pid attribute of inspect output
	// in containerd, pid is present inside `info.pid` of inspect output
	if runtime == "containerd" {
		var resp InspectResponse
		if err := json.Unmarshal(j, &resp); err != nil {
			return 0, err
		}
		pid = resp.Info.PID
	} else if runtime == "crio" {
		var resp InfoDetails
		if err := json.Unmarshal(j, &resp); err != nil {
			return 0, errors.Errorf("[cri]: Could not find pid field in json: %s", string(j))
		}
		pid = resp.PID
	} else {
		return 0, errors.Errorf("[cri]: No supported container runtime, runtime: %v", runtime)
	}

	if pid == 0 {
		return 0, errors.Errorf("[cri]: No running target container found, pid: %v", string(pid))
	}

	return pid, nil
}

// InjectChaos inject the network chaos in target container
// it is using nsenter command to enter into network namespace of target container
// and execute the netem command inside it.
func InjectChaos(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {

	netemCommands := os.Getenv("NETEM_COMMAND")
	targetIPs := os.Getenv("TARGET_IPs")

	if targetIPs == "" {
		tc := fmt.Sprintf("nsenter -t %d -n tc qdisc add dev %s root netem %v", pid, experimentDetails.NetworkInterface, netemCommands)
		cmd := exec.Command("/bin/bash", "-c", tc)
		out, err := cmd.CombinedOutput()
		log.Info(cmd.String())
		if err != nil {
			log.Error(string(out))
			return err
		}
	} else {

		ips := strings.Split(targetIPs, ",")
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
		priority := fmt.Sprintf("nsenter -t %v -n tc qdisc add dev %v root handle 1: prio", pid, experimentDetails.NetworkInterface)
		cmd := exec.Command("/bin/bash", "-c", priority)
		out, err := cmd.CombinedOutput()
		log.Info(cmd.String())
		if err != nil {
			log.Error(string(out))
			return err
		}

		// Add queueing discipline for 1:3 class.
		// No traffic is going through 1:3 yet
		traffic := fmt.Sprintf("nsenter -t %v -n tc qdisc add dev %v parent 1:3 netem %v", pid, experimentDetails.NetworkInterface, netemCommands)
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
				tc := fmt.Sprintf("nsenter -t %v -n tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip dst %v flowid 1:3", pid, experimentDetails.NetworkInterface, ip)
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
	return nil
}

// Killnetem kill the netem process for all the target containers
func Killnetem(PID int) error {

	tc := fmt.Sprintf("nsenter -t %d -n tc qdisc delete dev eth0 root", PID)
	cmd := exec.Command("/bin/bash", "-c", tc)
	out, err := cmd.CombinedOutput()
	log.Info(cmd.String())

	if err != nil {
		log.Error(string(out))
		return err
	}

	return nil
}

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = Getenv("EXPERIMENT_NAME", "")
	experimentDetails.AppNS = Getenv("APP_NS", "")
	experimentDetails.TargetContainer = Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPods = Getenv("APP_POD", "")
	experimentDetails.AppLabel = Getenv("APP_LABEL", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = Getenv("CHAOS_ENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = Getenv("POD_NAME", "")
	experimentDetails.ContainerRuntime = Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.NetworkInterface = Getenv("NETWORK_INTERFACE", "eth0")
	experimentDetails.TargetIPs = Getenv("TARGET_IPs", "")
}

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}
