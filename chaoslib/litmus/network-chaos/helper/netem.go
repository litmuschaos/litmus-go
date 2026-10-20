package helper

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/palantir/stacktrace"
	"go.opentelemetry.io/otel"

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
	err                                              error
	inject, abort                                    chan os.Signal
	sPorts, dPorts, whitelistDPorts, whitelistSPorts []string
)

// Helper injects the network chaos
func Helper(ctx context.Context, clients clients.ClientSets) {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "SimulatePodNetworkFault")
	defer span.End()

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

	err := preparePodNetworkChaos(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails)
	if err != nil {
		// update failstep inside chaosresult
		if resultErr := result.UpdateFailedStepFromHelper(&resultDetails, &chaosDetails, clients, err); resultErr != nil {
			log.Fatalf("helper pod failed, err: %v, resultErr: %v", err, resultErr)
		}
		log.Fatalf("helper pod failed, err: %v", err)
	}

}

// preparePodNetworkChaos contains the prepration steps before chaos injection
func preparePodNetworkChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {
	targetEnv := os.Getenv("TARGETS")
	if targetEnv == "" {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: chaosDetails.ChaosPodName, Reason: "no target found, provide atleast one target"}
	}

	var targets []targetDetails

	for _, t := range strings.Split(targetEnv, ";") {
		target := strings.Split(t, ":")
		if len(target) != 4 {
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: chaosDetails.ChaosPodName, Reason: fmt.Sprintf("unsupported target format: '%v'", targets)}
		}
		td := targetDetails{
			Name:            target[0],
			Namespace:       target[1],
			TargetContainer: target[2],
			DestinationIps:  getDestIps(target[3]),
			Source:          chaosDetails.ChaosPodName,
		}

		td.ContainerId, err = common.GetRuntimeBasedContainerID(experimentsDetails.ContainerRuntime, experimentsDetails.SocketPath, td.Name, td.Namespace, td.TargetContainer, clients, td.Source)
		if err != nil {
			return stacktrace.Propagate(err, "could not get container id")
		}

		// extract out the network ns path of the pod sandbox or pause container
		td.NetworkNsPath, err = common.GetNetworkNsPath(experimentsDetails.ContainerRuntime, td.ContainerId, experimentsDetails.SocketPath, td.Source)
		if err != nil {
			return stacktrace.Propagate(err, "could not get container network ns path")
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

	for index, t := range targets {
		// injecting network chaos inside target container
		if err = injectChaos(experimentsDetails.NetworkInterface, t); err != nil {
			if revertErr := revertChaosForAllTargets(targets, experimentsDetails.NetworkInterface, resultDetails, chaosDetails.ChaosNamespace, index-1); revertErr != nil {
				return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(revertErr).Error())}
			}
			return stacktrace.Propagate(err, "could not inject chaos")
		}
		log.Infof("successfully injected chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", t.Name); err != nil {
			if revertErr := revertChaosForAllTargets(targets, experimentsDetails.NetworkInterface, resultDetails, chaosDetails.ChaosNamespace, index); revertErr != nil {
				return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(revertErr).Error())}
			}
			return stacktrace.Propagate(err, "could not annotate chaosresult")
		}
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injected " + experimentsDetails.ExperimentName + " chaos on application pods"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	common.WaitForDuration(experimentsDetails.ChaosDuration)

	log.Info("[Chaos]: Duration is over, reverting chaos")

	if err := revertChaosForAllTargets(targets, experimentsDetails.NetworkInterface, resultDetails, chaosDetails.ChaosNamespace, len(targets)-1); err != nil {
		return stacktrace.Propagate(err, "could not revert chaos")
	}

	return nil
}

func revertChaosForAllTargets(targets []targetDetails, networkInterface string, resultDetails *types.ResultDetails, chaosNs string, index int) error {
	var errList []string
	for i := 0; i <= index; i++ {
		killed, err := killnetem(targets[i], networkInterface)
		if !killed && err != nil {
			errList = append(errList, err.Error())
			continue
		}
		if killed && err == nil {
			if err = result.AnnotateChaosResult(resultDetails.Name, chaosNs, "reverted", "pod", targets[i].Name); err != nil {
				errList = append(errList, err.Error())
			}
		}
	}

	if len(errList) != 0 {
		return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s]", strings.Join(errList, ","))}
	}
	return nil
}

// injectChaos inject the network chaos in target container
// it is using nsenter command to enter into network namespace of target container
// and execute the netem command inside it.
func injectChaos(netInterface string, target targetDetails) error {

	netemCommands := os.Getenv("NETEM_COMMAND")

	if len(target.DestinationIps) == 0 && len(sPorts) == 0 && len(dPorts) == 0 && len(whitelistDPorts) == 0 && len(whitelistSPorts) == 0 {
		tc := fmt.Sprintf("sudo nsenter --net=%s tc qdisc replace dev %s root %v", target.NetworkNsPath, netInterface, netemCommands)
		log.Info(tc)
		if err := common.RunBashCommand(tc, "failed to create tc rules", target.Source); err != nil {
			return err
		}
	} else {
		// Create a priority-based queue
		// This instantly creates classes 1:1, 1:2, 1:3
		priority := fmt.Sprintf("sudo nsenter --net=%s tc qdisc replace dev %v root handle 1: prio", target.NetworkNsPath, netInterface)
		log.Info(priority)
		if err := common.RunBashCommand(priority, "failed to create priority-based queue", target.Source); err != nil {
			return err
		}

		// Add queueing discipline for 1:3 class.
		// No traffic is going through 1:3 yet
		traffic := fmt.Sprintf("sudo nsenter --net=%s tc qdisc replace dev %v parent 1:3 %v", target.NetworkNsPath, netInterface, netemCommands)
		log.Info(traffic)
		if err := common.RunBashCommand(traffic, "failed to create netem queueing discipline", target.Source); err != nil {
			return err
		}

		if len(whitelistDPorts) != 0 || len(whitelistSPorts) != 0 {
			for _, port := range whitelistDPorts {
				//redirect traffic to specific dport through band 2
				tc := fmt.Sprintf("sudo nsenter --net=%s tc filter add dev %v protocol ip parent 1:0 prio 2 u32 match ip dport %v 0xffff flowid 1:2", target.NetworkNsPath, netInterface, port)
				log.Info(tc)
				if err := common.RunBashCommand(tc, "failed to create whitelist dport match filters", target.Source); err != nil {
					return err
				}
			}

			for _, port := range whitelistSPorts {
				//redirect traffic to specific sport through band 2
				tc := fmt.Sprintf("sudo nsenter --net=%s tc filter add dev %v protocol ip parent 1:0 prio 2 u32 match ip sport %v 0xffff flowid 1:2", target.NetworkNsPath, netInterface, port)
				log.Info(tc)
				if err := common.RunBashCommand(tc, "failed to create whitelist sport match filters", target.Source); err != nil {
					return err
				}
			}

			tc := fmt.Sprintf("sudo nsenter --net=%s tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip dst 0.0.0.0/0 flowid 1:3", target.NetworkNsPath, netInterface)
			log.Info(tc)
			if err := common.RunBashCommand(tc, "failed to create rule for all ports match filters", target.Source); err != nil {
				return err
			}
		} else {
			for i := range target.DestinationIps {
				var (
					ip     = target.DestinationIps[i]
					ports  []string
					isIPV6 = strings.Contains(target.DestinationIps[i], ":")
				)
				// extracting the destination ports from the ips
				// ip format is ip(|port1|port2....|portx)
				if strings.Contains(target.DestinationIps[i], "|") {
					ip = strings.Split(target.DestinationIps[i], "|")[0]
					ports = strings.Split(target.DestinationIps[i], "|")[1:]
				}

				// redirect traffic to specific IP through band 3
				filter := fmt.Sprintf("match ip dst %v", ip)
				if isIPV6 {
					filter = fmt.Sprintf("match ip6 dst %v", ip)
				}

				if len(ports) != 0 {
					for _, port := range ports {
						portFilter := fmt.Sprintf("%s match ip dport %v 0xffff", filter, port)
						if isIPV6 {
							portFilter = fmt.Sprintf("%s match ip6 dport %v 0xffff", filter, port)
						}

						tc := fmt.Sprintf("sudo nsenter --net=%s tc filter add dev %v protocol ip parent 1:0 prio 3 u32 %s flowid 1:3", target.NetworkNsPath, netInterface, portFilter)
						log.Info(tc)
						if err := common.RunBashCommand(tc, "failed to create destination ips match filters", target.Source); err != nil {
							return err
						}
					}
					continue
				}

				tc := fmt.Sprintf("sudo nsenter --net=%s tc filter add dev %v protocol ip parent 1:0 prio 3 u32 %s flowid 1:3", target.NetworkNsPath, netInterface, filter)
				log.Info(tc)
				if err := common.RunBashCommand(tc, "failed to create destination ips match filters", target.Source); err != nil {
					return err
				}
			}

			for _, port := range sPorts {
				//redirect traffic to specific sport through band 3
				tc := fmt.Sprintf("sudo nsenter --net=%s tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip sport %v 0xffff flowid 1:3", target.NetworkNsPath, netInterface, port)
				log.Info(tc)
				if err := common.RunBashCommand(tc, "failed to create source ports match filters", target.Source); err != nil {
					return err
				}
			}

			for _, port := range dPorts {
				//redirect traffic to specific dport through band 3
				tc := fmt.Sprintf("sudo nsenter --net=%s tc filter add dev %v protocol ip parent 1:0 prio 3 u32 match ip dport %v 0xffff flowid 1:3", target.NetworkNsPath, netInterface, port)
				log.Info(tc)
				if err := common.RunBashCommand(tc, "failed to create destination ports match filters", target.Source); err != nil {
					return err
				}
			}
		}
	}

	log.Infof("chaos injected successfully on {pod: %v, container: %v}", target.Name, target.TargetContainer)
	return nil
}

// killnetem kill the netem process for all the target containers
func killnetem(target targetDetails, networkInterface string) (bool, error) {
	tc := fmt.Sprintf("sudo nsenter --net=%s tc qdisc delete dev %s root", target.NetworkNsPath, networkInterface)
	cmd := exec.Command("/bin/bash", "-c", tc)
	out, err := cmd.CombinedOutput()

	if err != nil {
		log.Info(cmd.String())
		// ignoring err if qdisc process doesn't exist inside the target container
		if strings.Contains(string(out), qdiscNotFound) || strings.Contains(string(out), qdiscNoFileFound) {
			log.Warn("The network chaos process has already been removed")
			return true, err
		}
		log.Error(err.Error())
		return false, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Source: target.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", target.Name, target.Namespace, target.TargetContainer), Reason: fmt.Sprintf("failed to revert network faults: %s", string(out))}
	}
	log.Infof("successfully reverted chaos on target: {name: %s, namespace: %v, container: %v}", target.Name, target.Namespace, target.TargetContainer)
	return true, nil
}

type targetDetails struct {
	Name            string
	Namespace       string
	ServiceMesh     string
	DestinationIps  []string
	TargetContainer string
	ContainerId     string
	Source          string
	NetworkNsPath   string
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
	experimentDetails.NetworkInterface = types.Getenv("NETWORK_INTERFACE", "")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "")
	experimentDetails.DestinationIPs = types.Getenv("DESTINATION_IPS", "")
	experimentDetails.SourcePorts = types.Getenv("SOURCE_PORTS", "")
	experimentDetails.DestinationPorts = types.Getenv("DESTINATION_PORTS", "")

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
func getDestIps(serviceMesh string) []string {
	var (
		destIps   = os.Getenv("DESTINATION_IPS")
		uniqueIps []string
	)

	if serviceMesh == "true" {
		destIps = os.Getenv("DESTINATION_IPS_SERVICE_MESH")
	}

	if strings.TrimSpace(destIps) == "" {
		return nil
	}

	ips := strings.Split(strings.TrimSpace(destIps), ",")

	// removing duplicates ips from the list, if any
	for i := range ips {
		if !common.Contains(ips[i], uniqueIps) {
			uniqueIps = append(uniqueIps, ips[i])
		}
	}

	return uniqueIps
}
