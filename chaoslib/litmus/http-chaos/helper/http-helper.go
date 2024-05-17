package helper

import (
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
	"os"
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

	// Initialise the chaos attributes
	types.InitialiseChaosVariables(&chaosDetails)
	chaosDetails.Phase = types.ChaosInjectPhase

	// Initialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	err := prepareK8sHttpChaos(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails)
	if err != nil {
		// update failstep inside chaosresult
		if resultErr := result.UpdateFailedStepFromHelper(&resultDetails, &chaosDetails, clients, err); resultErr != nil {
			log.Fatalf("helper pod failed, err: %v, resultErr: %v", err, resultErr)
		}
		log.Fatalf("helper pod failed, err: %v", err)
	}
}

// prepareK8sHttpChaos contains the preparation steps before chaos injection
func prepareK8sHttpChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

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

		td.ContainerId, err = common.GetRuntimeBasedContainerID(experimentsDetails.ContainerRuntime, experimentsDetails.SocketPath, td.Name, td.Namespace, td.TargetContainer, clients, td.Source)
		if err != nil {
			return stacktrace.Propagate(err, "could not get container id")
		}

		// extract out the pid of the target container
		td.Pid, err = common.GetPauseAndSandboxPID(experimentsDetails.ContainerRuntime, td.ContainerId, experimentsDetails.SocketPath, td.Source)
		if err != nil {
			return stacktrace.Propagate(err, "could not get container pid")
		}
		targets = append(targets, td)
	}

	// watching for the abort signal and revert the chaos
	go abortWatcher(targets, resultDetails.Name, chaosDetails.ChaosNamespace, experimentsDetails)

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(1)
	default:
	}

	for _, t := range targets {
		// injecting http chaos inside target container
		if err = injectChaos(experimentsDetails, t); err != nil {
			return stacktrace.Propagate(err, "could not inject chaos")
		}
		log.Infof("successfully injected chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", t.Name); err != nil {
			if revertErr := revertChaos(experimentsDetails, t); revertErr != nil {
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

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	common.WaitForDuration(experimentsDetails.ChaosDuration)

	log.Info("[Chaos]: chaos duration is over, reverting chaos")

	var errList []string
	for _, t := range targets {
		// cleaning the ip rules process after chaos injection
		err := revertChaos(experimentsDetails, t)
		if err != nil {
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
	return nil
}

// injectChaos inject the http chaos in target container and add ruleset to the iptables to redirect the ports
func injectChaos(experimentDetails *experimentTypes.ExperimentDetails, t targetDetails) error {
	if err := startProxy(experimentDetails, t.Pid); err != nil {
		killErr := killProxy(t.Pid, t.Source)
		if killErr != nil {
			return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(killErr).Error())}
		}
		return stacktrace.Propagate(err, "could not start proxy server")
	}
	if err := addIPRuleSet(experimentDetails, t.Pid); err != nil {
		killErr := killProxy(t.Pid, t.Source)
		if killErr != nil {
			return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(killErr).Error())}
		}
		return stacktrace.Propagate(err, "could not add ip rules")
	}
	return nil
}

// revertChaos revert the http chaos in target container
func revertChaos(experimentDetails *experimentTypes.ExperimentDetails, t targetDetails) error {

	var errList []string

	if err := removeIPRuleSet(experimentDetails, t.Pid); err != nil {
		errList = append(errList, err.Error())
	}

	if err := killProxy(t.Pid, t.Source); err != nil {
		errList = append(errList, err.Error())
	}
	if len(errList) != 0 {
		return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s]", strings.Join(errList, ","))}
	}
	log.Infof("successfully reverted chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
	return nil
}

// startProxy starts the proxy process inside the target container
// it is using nsenter command to enter into network namespace of target container
// and execute the proxy related command inside it.
func startProxy(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {

	toxics := os.Getenv("TOXIC_COMMAND")

	// starting toxiproxy server inside the target container
	startProxyServerCommand := fmt.Sprintf("(sudo nsenter -t %d -n toxiproxy-server -host=0.0.0.0 > /dev/null 2>&1 &)", pid)
	// Creating a proxy for the targeted service in the target container
	createProxyCommand := fmt.Sprintf("(sudo nsenter -t %d -n toxiproxy-cli create -l 0.0.0.0:%d -u 0.0.0.0:%d proxy)", pid, experimentDetails.ProxyPort, experimentDetails.TargetServicePort)
	createToxicCommand := fmt.Sprintf("(sudo nsenter -t %d -n toxiproxy-cli toxic add %s --toxicity %f proxy)", pid, toxics, float32(experimentDetails.Toxicity)/100.0)

	// sleep 2 is added for proxy-server to be ready for creating proxy and adding toxics
	chaosCommand := fmt.Sprintf("%s && sleep 2 && %s && %s", startProxyServerCommand, createProxyCommand, createToxicCommand)

	log.Infof("[Chaos]: Starting proxy server")

	if err := common.RunBashCommand(chaosCommand, "failed to start proxy server", experimentDetails.ChaosPodName); err != nil {
		return err
	}

	log.Info("[Info]: Proxy started successfully")
	return nil
}

const NoProxyToKill = "you need to specify whom to kill"

// killProxy kills the proxy process inside the target container
// it is using nsenter command to enter into network namespace of target container
// and execute the proxy related command inside it.
func killProxy(pid int, source string) error {
	stopProxyServerCommand := fmt.Sprintf("sudo nsenter -t %d -n sudo kill -9 $(ps aux | grep [t]oxiproxy | awk 'FNR==1{print $1}')", pid)
	log.Infof("[Chaos]: Stopping proxy server")

	if err := common.RunBashCommand(stopProxyServerCommand, "failed to stop proxy server", source); err != nil {
		return err
	}

	log.Info("[Info]: Proxy stopped successfully")
	return nil
}

// addIPRuleSet adds the ip rule set to iptables in target container
// it is using nsenter command to enter into network namespace of target container
// and execute the iptables related command inside it.
func addIPRuleSet(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {
	// it adds the proxy port REDIRECT iprule in the beginning of the PREROUTING table
	// so that it always matches all the incoming packets for the matching target port filters and
	// if matches then it redirect the request to the proxy port
	addIPRuleSetCommand := fmt.Sprintf("(sudo nsenter -t %d -n iptables -t nat -I PREROUTING -i %v -p tcp --dport %d -j REDIRECT --to-port %d)", pid, experimentDetails.NetworkInterface, experimentDetails.TargetServicePort, experimentDetails.ProxyPort)
	log.Infof("[Chaos]: Adding IPtables ruleset")

	if err := common.RunBashCommand(addIPRuleSetCommand, "failed to add ip rules", experimentDetails.ChaosPodName); err != nil {
		return err
	}

	log.Info("[Info]: IP rule set added successfully")
	return nil
}

const NoIPRulesetToRemove = "No chain/target/match by that name"

// removeIPRuleSet removes the ip rule set from iptables in target container
// it is using nsenter command to enter into network namespace of target container
// and execute the iptables related command inside it.
func removeIPRuleSet(experimentDetails *experimentTypes.ExperimentDetails, pid int) error {
	removeIPRuleSetCommand := fmt.Sprintf("sudo nsenter -t %d -n iptables -t nat -D PREROUTING -i %v -p tcp --dport %d -j REDIRECT --to-port %d", pid, experimentDetails.NetworkInterface, experimentDetails.TargetServicePort, experimentDetails.ProxyPort)
	log.Infof("[Chaos]: Removing IPtables ruleset")

	if err := common.RunBashCommand(removeIPRuleSetCommand, "failed to remove ip rules", experimentDetails.ChaosPodName); err != nil {
		return err
	}

	log.Info("[Info]: IP rule set removed successfully")
	return nil
}

// getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", ""))
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "")
	experimentDetails.NetworkInterface = types.Getenv("NETWORK_INTERFACE", "")
	experimentDetails.TargetServicePort, _ = strconv.Atoi(types.Getenv("TARGET_SERVICE_PORT", ""))
	experimentDetails.ProxyPort, _ = strconv.Atoi(types.Getenv("PROXY_PORT", ""))
	experimentDetails.Toxicity, _ = strconv.Atoi(types.Getenv("TOXICITY", "100"))
}

// abortWatcher continuously watch for the abort signals
func abortWatcher(targets []targetDetails, resultName, chaosNS string, experimentDetails *experimentTypes.ExperimentDetails) {

	<-abort
	log.Info("[Abort]: Killing process started because of terminated signal received")
	log.Info("[Abort]: Chaos Revert Started")

	retry := 3
	for retry > 0 {
		for _, t := range targets {
			if err = revertChaos(experimentDetails, t); err != nil {
				if strings.Contains(err.Error(), NoIPRulesetToRemove) && strings.Contains(err.Error(), NoProxyToKill) {
					continue
				}
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

	log.Info("Chaos Revert Completed")
	os.Exit(1)
}

type targetDetails struct {
	Name            string
	Namespace       string
	TargetContainer string
	ContainerId     string
	Pid             int
	Source          string
}
