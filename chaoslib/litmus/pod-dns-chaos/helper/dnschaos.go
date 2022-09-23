package helper

import (
	"fmt"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-dns-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

var (
	abort, injectAbort chan os.Signal
)

var err error

const (
	// ProcessAlreadyFinished contains error code when process is finished
	ProcessAlreadyFinished = "os: process already finished"
	// ProcessAlreadyKilled contains error code when process is already killed
	ProcessAlreadyKilled = "no such process"
)

// Helper injects the dns chaos
func Helper(clients clients.ClientSets) {

	experimentsDetails := experimentTypes.ExperimentDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}
	resultDetails := types.ResultDetails{}

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// injectAbort channel is used to transmit signal notifications.
	injectAbort = make(chan os.Signal, 1)

	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)
	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(injectAbort, os.Interrupt, syscall.SIGTERM)

	//Fetching all the ENV passed for the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	getENV(&experimentsDetails)

	// Initialise the chaos attributes
	types.InitialiseChaosVariables(&chaosDetails)

	// Initialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	if err := preparePodDNSChaos(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails); err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}

}

//preparePodDNSChaos contains the preparation steps before chaos injection
func preparePodDNSChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	targetEnv := os.Getenv("TARGETS")
	if targetEnv == "" {
		return fmt.Errorf("no target found, provide atleast one target")
	}

	var targets []targetDetails

	for _, t := range strings.Split(targetEnv, ";") {
		target := strings.Split(t, ":")
		if len(target) != 3 {
			return fmt.Errorf("unsupported target: '%v', provide target in '<name>:<namespace>:<containerName>", target)
		}
		td := targetDetails{
			Name:            target[0],
			Namespace:       target[1],
			TargetContainer: target[2],
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

	// watching for the abort signal and revert the chaos if an abort signal is received
	go abortWatcher(targets, resultDetails.Name, chaosDetails.ChaosNamespace)

	select {
	case <-injectAbort:
		// stopping the chaos execution, if abort signal received
		os.Exit(1)
	default:
	}

	done := make(chan bool, 1)

	for index, t := range targets {
		targets[index].Cmd, err = injectChaos(experimentsDetails, t.Pid)
		if err != nil {
			return err
		}
		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", t.Name); err != nil {
			if revertErr := terminateProcess(t.Cmd); revertErr != nil {
				return fmt.Errorf("failed to revert and annotate the result, err: %v", fmt.Sprintf("%s, %s", err.Error(), revertErr.Error()))
			}
			return err
		}
	}

	// record the event inside chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	log.Info("[Wait]: Waiting for chaos completion")
	// channel to check the completion of the stress process
	go func() {
		var errList []string
		for _, t := range targets {
			if err := t.Cmd.Wait(); err != nil {
				log.Errorf("err -- %v", err.Error())
				errList = append(errList, err.Error())
			}
		}
		if len(errList) != 0 {
			log.Errorf("err: %v", strings.Join(errList, ", "))
		}
		done <- true
	}()

	// check the timeout for the command
	// Note: timeout will occur when process didn't complete even after 10s of chaos duration
	timeout := time.After((time.Duration(experimentsDetails.ChaosDuration) + 30) * time.Second)

	select {
	case <-timeout:
		// the stress process gets timeout before completion
		log.Infof("[Chaos] The stress process is not yet completed after the chaos duration of %vs", experimentsDetails.ChaosDuration+30)
		log.Info("[Timeout]: Killing the stress process")
		var errList []string
		for _, t := range targets {
			if err = terminateProcess(t.Cmd); err != nil {
				errList = append(errList, err.Error())
				continue
			}
			if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", t.Name); err != nil {
				errList = append(errList, err.Error())
			}
		}
		if len(errList) != 0 {
			return fmt.Errorf("err: %v", strings.Join(errList, ", "))
		}
	case <-done:
		select {
		case <-injectAbort:
			// wait for the completion of abort handler
			time.Sleep(10 * time.Second)
		default:
			log.Info("[Info]: Reverting Chaos")
			var errList []string
			for _, t := range targets {
				if err := terminateProcess(t.Cmd); err != nil {
					errList = append(errList, err.Error())
					continue
				}
				if err := result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", t.Name); err != nil {
					errList = append(errList, err.Error())
				}
			}
			if len(errList) != 0 {
				return fmt.Errorf("err: %v", strings.Join(errList, ", "))
			}
		}
	}

	return nil
}

func injectChaos(experimentsDetails *experimentTypes.ExperimentDetails, pid int) (*exec.Cmd, error) {

	// prepare dns interceptor
	commandTemplate := fmt.Sprintf("sudo TARGET_PID=%d CHAOS_TYPE=%s SPOOF_MAP='%s' TARGET_HOSTNAMES='%s' CHAOS_DURATION=%d MATCH_SCHEME=%s nsutil -p -n -t %d -- dns_interceptor", pid, experimentsDetails.ChaosType, experimentsDetails.SpoofMap, experimentsDetails.TargetHostNames, experimentsDetails.ChaosDuration, experimentsDetails.MatchScheme, pid)
	cmd := exec.Command("/bin/bash", "-c", commandTemplate)
	log.Info(cmd.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return nil, errors.Errorf("fail to start the dns process, err: %v", err)
	}
	return cmd, nil
}

func terminateProcess(cmd *exec.Cmd) error {
	// kill command
	killTemplate := fmt.Sprintf("sudo kill %d", cmd.Process.Pid)
	kill := exec.Command("/bin/bash", "-c", killTemplate)
	if err = kill.Run(); err != nil {
		log.Errorf("unable to kill dns interceptor process cry, err :%v", err)
	} else {
		log.Errorf("dns interceptor process stopped")
	}
	return nil
}

// abortWatcher continuously watch for the abort signals
func abortWatcher(targets []targetDetails, resultName, chaosNS string) {

	<-abort

	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("[Abort]: Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		for _, t := range targets {
			if err = terminateProcess(t.Cmd); err != nil {
				log.Errorf("unable to revert for %v pod, err :%v", t.Name, err)
				continue
			}
			if err = result.AnnotateChaosResult(resultName, chaosNS, "reverted", "pod", t.Name); err != nil {
				log.Errorf("unable to annotate the chaosresult for %v pod, err :%v", t.Name, err)
			}
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}

//getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "60"))
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.TargetHostNames = types.Getenv("TARGET_HOSTNAMES", "")
	experimentDetails.SpoofMap = types.Getenv("SPOOF_MAP", "")
	experimentDetails.MatchScheme = types.Getenv("MATCH_SCHEME", "exact")
	experimentDetails.ChaosType = types.Getenv("CHAOS_TYPE", "error")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "")
}

type targetDetails struct {
	Name            string
	Namespace       string
	TargetContainer string
	ContainerId     string
	Pid             int
	CommandPid      int
	Cmd             *exec.Cmd
}
