package helper

import (
	"bytes"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
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
	err                error
)

const (
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
		// update failstep inside chaosresult
		if resultErr := result.UpdateFailedStepFromHelper(&resultDetails, &chaosDetails, clients, err); resultErr != nil {
			log.Fatalf("helper pod failed, err: %v, resultErr: %v", err, resultErr)
		}
		log.Fatalf("helper pod failed, err: %v", err)
	}

}

//preparePodDNSChaos contains the preparation steps before chaos injection
func preparePodDNSChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	targetList, err := common.ParseTargets(chaosDetails.ChaosPodName)
	if err != nil {
		return stacktrace.Propagate(err, "could not parse targets")
	}

	var targets []targetDetails

	for _, t := range targetList.Target {
		td := targetDetails{
			Name:            t.Name,
			Namespace:       t.Namespace,
			TargetContainer: t.TargetContainer,
			Source:          chaosDetails.ChaosPodName,
		}

		td.ContainerId, err = common.GetContainerID(td.Namespace, td.Name, td.TargetContainer, clients, td.Source)
		if err != nil {
			return stacktrace.Propagate(err, "could not get container id")
		}

		// extract out the pid of the target container
		td.Pid, err = common.GetPID(experimentsDetails.ContainerRuntime, td.ContainerId, experimentsDetails.SocketPath, td.Source)
		if err != nil {
			return stacktrace.Propagate(err, "could not get container pid")
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

	done := make(chan error, 1)

	for index, t := range targets {
		targets[index].Cmd, err = injectChaos(experimentsDetails, t)
		if err != nil {
			return stacktrace.Propagate(err, "could not inject chaos")
		}
		log.Infof("successfully injected chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", t.Name); err != nil {
			if revertErr := terminateProcess(t); revertErr != nil {
				return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(revertErr).Error())}
			}
			return stacktrace.Propagate(err, "could not annotate chaosresult")
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
				errList = append(errList, err.Error())
			}
		}
		if len(errList) != 0 {
			log.Errorf("err: %v", strings.Join(errList, ", "))
			done <- fmt.Errorf("err: %v", strings.Join(errList, ", "))
		}
		done <- nil
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
			if err = terminateProcess(t); err != nil {
				errList = append(errList, err.Error())
				continue
			}
			if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", t.Name); err != nil {
				errList = append(errList, err.Error())
			}
		}
		if len(errList) != 0 {
			return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s]", strings.Join(errList, ","))}
		}
	case doneErr := <-done:
		select {
		case <-injectAbort:
			// wait for the completion of abort handler
			time.Sleep(10 * time.Second)
		default:
			log.Info("[Info]: Reverting Chaos")
			var errList []string
			for _, t := range targets {
				if err := terminateProcess(t); err != nil {
					errList = append(errList, err.Error())
					continue
				}
				if err := result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", t.Name); err != nil {
					errList = append(errList, err.Error())
				}
			}
			if len(errList) != 0 {
				return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s]", strings.Join(errList, ","))}
			}
			return doneErr
		}
	}

	return nil
}

func injectChaos(experimentsDetails *experimentTypes.ExperimentDetails, t targetDetails) (*exec.Cmd, error) {

	// prepare dns interceptor
	var out bytes.Buffer
	commandTemplate := fmt.Sprintf("sudo TARGET_PID=%d CHAOS_TYPE=%s SPOOF_MAP='%s' TARGET_HOSTNAMES='%s' CHAOS_DURATION=%d MATCH_SCHEME=%s nsutil -p -n -t %d -- dns_interceptor", t.Pid, experimentsDetails.ChaosType, experimentsDetails.SpoofMap, experimentsDetails.TargetHostNames, experimentsDetails.ChaosDuration, experimentsDetails.MatchScheme, t.Pid)
	cmd := exec.Command("/bin/bash", "-c", commandTemplate)
	log.Info(cmd.String())
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err = cmd.Start(); err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: experimentsDetails.ChaosPodName, Target: fmt.Sprintf("{podName: %s, namespace: %s}", t.Name, t.Namespace), Reason: fmt.Sprintf("faild to inject chaos: %s", out.String())}
	}
	return cmd, nil
}

func terminateProcess(t targetDetails) error {
	// kill command
	killTemplate := fmt.Sprintf("sudo kill %d", t.Cmd.Process.Pid)
	kill := exec.Command("/bin/bash", "-c", killTemplate)
	var out bytes.Buffer
	kill.Stderr = &out
	kill.Stdout = &out
	if err = kill.Run(); err != nil {
		if strings.Contains(strings.ToLower(out.String()), ProcessAlreadyKilled) {
			return nil
		}
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s}", t.Name, t.Namespace), Reason: fmt.Sprintf("failed to revert chaos %s", out.String())}
	} else {
		log.Errorf("dns interceptor process stopped")
		log.Infof("successfully injected chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
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
			if err = terminateProcess(t); err != nil {
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
	Source          string
}
