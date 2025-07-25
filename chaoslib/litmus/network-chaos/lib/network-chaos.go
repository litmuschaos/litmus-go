package lib

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/palantir/stacktrace"
	"go.opentelemetry.io/otel"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var serviceMesh = []string{"istio", "envoy"}
var destIpsSvcMesh string
var destIps string

// PrepareAndInjectChaos contains the preparation & injection steps
func PrepareAndInjectChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, args string) error {

	var err error
	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail == nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "provide one of the appLabel or TARGET_PODS"}
	}
	//set up the tunables if provided in range
	SetChaosTunables(experimentsDetails)
	logExperimentFields(experimentsDetails)

	targetPodList, err := common.GetTargetPods(experimentsDetails.NodeLabel, experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
	if err != nil {
		return stacktrace.Propagate(err, "could not get target pods")
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// Getting the serviceAccountName, need permission inside helper pod to create the events
	if experimentsDetails.ChaosServiceAccount == "" {
		experimentsDetails.ChaosServiceAccount, err = common.GetServiceAccount(experimentsDetails.ChaosNamespace, experimentsDetails.ChaosPodName, clients)
		if err != nil {
			return stacktrace.Propagate(err, "could not get experiment service account")
		}
	}

	if experimentsDetails.EngineName != "" {
		if err := common.SetHelperData(chaosDetails, experimentsDetails.SetHelperData, clients); err != nil {
			return stacktrace.Propagate(err, "could not set helper data")
		}
	}

	experimentsDetails.IsTargetContainerProvided = experimentsDetails.TargetContainer != ""
	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(ctx, experimentsDetails, targetPodList, clients, chaosDetails, args, resultDetails, eventsDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err = injectChaosInParallelMode(ctx, experimentsDetails, targetPodList, clients, chaosDetails, args, resultDetails, eventsDetails); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
	}

	return nil
}

// injectChaosInSerialMode inject the network chaos in all target application serially (one by one)
func injectChaosInSerialMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, args string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectPodNetworkFaultInSerialMode")
	defer span.End()

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// creating the helper pod to perform network chaos
	for _, pod := range targetPodList.Items {

		serviceMesh, err := setDestIps(pod, experimentsDetails, clients)
		if err != nil {
			return stacktrace.Propagate(err, "could not set destination ips")
		}

		//Get the target container name of the application pod
		if !experimentsDetails.IsTargetContainerProvided {
			experimentsDetails.TargetContainer = pod.Spec.Containers[0].Name
		}

		runID := stringutils.GetRunID()

		if err := createHelperPod(ctx, experimentsDetails, clients, chaosDetails, fmt.Sprintf("%s:%s:%s:%s", pod.Name, pod.Namespace, experimentsDetails.TargetContainer, serviceMesh), pod.Spec.NodeName, runID, args); err != nil {
			return stacktrace.Propagate(err, "could not create helper pod")
		}

		appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, runID)

		//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
		if err := common.ManagerHelperLifecycle(appLabel, chaosDetails, clients, true); err != nil {
			return err
		}
	}

	return nil
}

// injectChaosInParallelMode inject the network chaos in all target application in parallel mode (all at once)
func injectChaosInParallelMode(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, args string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "InjectPodNetworkFaultInParallelMode")
	defer span.End()
	var err error

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(ctx, chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	targets, err := filterPodsForNodes(targetPodList, experimentsDetails, clients)
	if err != nil {
		return stacktrace.Propagate(err, "could not filter target pods")
	}

	runID := stringutils.GetRunID()

	for node, tar := range targets {
		var targetsPerNode []string
		for _, k := range tar.Target {
			targetsPerNode = append(targetsPerNode, fmt.Sprintf("%s:%s:%s:%s", k.Name, k.Namespace, k.TargetContainer, k.ServiceMesh))
		}

		if err := createHelperPod(ctx, experimentsDetails, clients, chaosDetails, strings.Join(targetsPerNode, ";"), node, runID, args); err != nil {
			return stacktrace.Propagate(err, "could not create helper pod")
		}
	}

	appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, runID)

	//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pods")
	if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return stacktrace.Propagate(err, "could not check helper status")
	}

	// Wait till the completion of the helper pod
	// set an upper limit for the waiting time
	log.Info("[Wait]: waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, common.GetContainerNames(chaosDetails)...)
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return common.HelperFailedError(err, appLabel, chaosDetails.ChaosNamespace, true)
	}

	//Deleting all the helper pod for container-kill chaos
	log.Info("[Cleanup]: Deleting all the helper pod")
	if err := common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return stacktrace.Propagate(err, "could not delete helper pod(s)")
	}

	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, targets string, nodeName, runID, args string) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "CreatePodNetworkFaultHelperPod")
	defer span.End()

	var (
		privilegedEnable              = true
		terminationGracePeriodSeconds = int64(experimentsDetails.TerminationGracePeriodSeconds)
		helperName                    = fmt.Sprintf("%s-helper-%s", experimentsDetails.ExperimentName, stringutils.GetRunID())
	)

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        helperName,
			Namespace:   experimentsDetails.ChaosNamespace,
			Labels:      common.GetHelperLabels(chaosDetails.Labels, runID, experimentsDetails.ExperimentName),
			Annotations: chaosDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			HostPID:                       true,
			TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
			ImagePullSecrets:              chaosDetails.ImagePullSecrets,
			Tolerations:                   chaosDetails.Tolerations,
			ServiceAccountName:            experimentsDetails.ChaosServiceAccount,
			RestartPolicy:                 apiv1.RestartPolicyNever,
			NodeName:                      nodeName,
			Volumes: []apiv1.Volume{
				{
					Name: "cri-socket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: experimentsDetails.SocketPath,
						},
					},
				},
			},

			Containers: []apiv1.Container{
				{
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"/bin/bash",
					},
					Args: []string{
						"-c",
						"./helpers -name network-chaos",
					},
					Resources: chaosDetails.Resources,
					Env:       getPodEnv(ctx, experimentsDetails, targets, args),
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "cri-socket",
							MountPath: experimentsDetails.SocketPath,
						},
					},
					SecurityContext: &apiv1.SecurityContext{
						Privileged: &privilegedEnable,
						Capabilities: &apiv1.Capabilities{
							Add: []apiv1.Capability{
								"NET_ADMIN",
								"SYS_ADMIN",
							},
						},
					},
				},
			},
		},
	}

	if len(chaosDetails.SideCar) != 0 {
		helperPod.Spec.Containers = append(helperPod.Spec.Containers, common.BuildSidecar(chaosDetails)...)
		helperPod.Spec.Volumes = append(helperPod.Spec.Volumes, common.GetSidecarVolumes(chaosDetails)...)
	}

	// mount the network ns path for crio runtime
	// it is required to access the sandbox network ns
	if strings.ToLower(experimentsDetails.ContainerRuntime) == "crio" {
		helperPod.Spec.Volumes = append(helperPod.Spec.Volumes, apiv1.Volume{
			Name: "netns-path",
			VolumeSource: apiv1.VolumeSource{
				HostPath: &apiv1.HostPathVolumeSource{
					Path: "/var/run/netns",
				},
			},
		})
		helperPod.Spec.Containers[0].VolumeMounts = append(helperPod.Spec.Containers[0].VolumeMounts, apiv1.VolumeMount{
			Name:      "netns-path",
			MountPath: "/var/run/netns",
		})
	}

	if err := clients.CreatePod(experimentsDetails.ChaosNamespace, helperPod); err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("unable to create helper pod: %s", err.Error())}
	}

	return nil
}

// getPodEnv derive all the env required for the helper pod
func getPodEnv(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, targets string, args string) []apiv1.EnvVar {

	var envDetails common.ENVDetails
	envDetails.SetEnv("TARGETS", targets).
		SetEnv("TOTAL_CHAOS_DURATION", strconv.Itoa(experimentsDetails.ChaosDuration)).
		SetEnv("CHAOS_NAMESPACE", experimentsDetails.ChaosNamespace).
		SetEnv("CHAOSENGINE", experimentsDetails.EngineName).
		SetEnv("CHAOS_UID", string(experimentsDetails.ChaosUID)).
		SetEnv("CONTAINER_RUNTIME", experimentsDetails.ContainerRuntime).
		SetEnv("NETEM_COMMAND", args).
		SetEnv("NETWORK_INTERFACE", experimentsDetails.NetworkInterface).
		SetEnv("EXPERIMENT_NAME", experimentsDetails.ExperimentName).
		SetEnv("SOCKET_PATH", experimentsDetails.SocketPath).
		SetEnv("INSTANCE_ID", experimentsDetails.InstanceID).
		SetEnv("DESTINATION_IPS", destIps).
		SetEnv("DESTINATION_IPS_SERVICE_MESH", destIpsSvcMesh).
		SetEnv("SOURCE_PORTS", experimentsDetails.SourcePorts).
		SetEnv("DESTINATION_PORTS", experimentsDetails.DestinationPorts).
		SetEnv("OTEL_EXPORTER_OTLP_ENDPOINT", os.Getenv(telemetry.OTELExporterOTLPEndpoint)).
		SetEnv("TRACE_PARENT", telemetry.GetMarshalledSpanFromContext(ctx)).
		SetEnvFromDownwardAPI("v1", "metadata.name")

	return envDetails.ENV
}

type targetsDetails struct {
	Target []target
}

type target struct {
	Namespace       string
	Name            string
	TargetContainer string
	ServiceMesh     string
}

// GetTargetIps return the comma separated target ips
// It fetches the ips from the target ips (if defined by users)
// it appends the ips from the host, if target host is provided
func GetTargetIps(targetIPs, targetHosts string, clients clients.ClientSets, serviceMesh bool) (string, error) {

	ipsFromHost, err := getIpsForTargetHosts(targetHosts, clients, serviceMesh)
	if err != nil {
		return "", stacktrace.Propagate(err, "could not get ips from target hosts")
	}
	if targetIPs == "" {
		targetIPs = ipsFromHost
	} else if ipsFromHost != "" {
		targetIPs = targetIPs + "," + ipsFromHost
	}
	return targetIPs, nil
}

// it derives the pod ips from the kubernetes service
func getPodIPFromService(host string, clients clients.ClientSets) ([]string, error) {
	var ips []string
	svcFields := strings.Split(host, ".")
	if len(svcFields) != 5 {
		return ips, cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{host: %s}", host), Reason: "provide the valid FQDN for service in '<svc-name>.<namespace>.svc.cluster.local format"}
	}
	svcName, svcNs := svcFields[0], svcFields[1]
	svc, err := clients.GetService(svcNs, svcName)
	if err != nil {
		if k8serrors.IsForbidden(err) {
			log.Warnf("forbidden - failed to get %v service in %v namespace, err: %v", svcName, svcNs, err)
			return ips, nil
		}
		return ips, cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{serviceName: %s, namespace: %s}", svcName, svcNs), Reason: err.Error()}
	}

	if svc.Spec.Selector == nil {
		return nil, nil
	}
	var svcSelector string
	for k, v := range svc.Spec.Selector {
		if svcSelector == "" {
			svcSelector += fmt.Sprintf("%s=%s", k, v)
			continue
		}
		svcSelector += fmt.Sprintf(",%s=%s", k, v)
	}

	pods, err := clients.ListPods(svcNs, svcSelector)
	if err != nil {
		return ips, cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{svcName: %s,podLabel: %s, namespace: %s}", svcNs, svcSelector, svcNs), Reason: fmt.Sprintf("failed to derive pods from service: %s", err.Error())}
	}
	for _, p := range pods.Items {
		if p.Status.PodIP == "" {
			continue
		}
		ips = append(ips, p.Status.PodIP)
	}

	return ips, nil
}

// getIpsForTargetHosts resolves IP addresses for comma-separated list of target hosts and returns comma-separated ips
func getIpsForTargetHosts(targetHosts string, clients clients.ClientSets, serviceMesh bool) (string, error) {
	if targetHosts == "" {
		return "", nil
	}
	hosts := strings.Split(targetHosts, ",")
	finalHosts := ""
	var commaSeparatedIPs []string
	for i := range hosts {
		hosts[i] = strings.TrimSpace(hosts[i])
		var (
			hostName = hosts[i]
			ports    []string
		)

		if strings.Contains(hosts[i], "|") {
			host := strings.Split(hosts[i], "|")
			hostName = host[0]
			ports = host[1:]
			log.Infof("host and port: %v :%v", hostName, ports)
		}

		if strings.Contains(hostName, "svc.cluster.local") && serviceMesh {
			ips, err := getPodIPFromService(hostName, clients)
			if err != nil {
				return "", stacktrace.Propagate(err, "could not get pod ips from service")
			}
			log.Infof("Host: {%v}, IP address: {%v}", hosts[i], ips)
			if ports != nil {
				for j := range ips {
					commaSeparatedIPs = append(commaSeparatedIPs, ips[j]+"|"+strings.Join(ports, "|"))
				}
			} else {
				commaSeparatedIPs = append(commaSeparatedIPs, ips...)
			}

			if finalHosts == "" {
				finalHosts = hosts[i]
			} else {
				finalHosts = finalHosts + "," + hosts[i]
			}
			continue
		}
		ips, err := net.LookupIP(hostName)
		if err != nil {
			log.Warnf("Unknown host: {%v}, it won't be included in the scope of chaos", hostName)
		} else {
			for j := range ips {
				log.Infof("Host: {%v}, IP address: {%v}", hostName, ips[j])
				if ports != nil {
					commaSeparatedIPs = append(commaSeparatedIPs, ips[j].String()+"|"+strings.Join(ports, "|"))
					continue
				}
				commaSeparatedIPs = append(commaSeparatedIPs, ips[j].String())
			}
			if finalHosts == "" {
				finalHosts = hosts[i]
			} else {
				finalHosts = finalHosts + "," + hosts[i]
			}
		}
	}
	if len(commaSeparatedIPs) == 0 {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("hosts: %s", targetHosts), Reason: "provided hosts are invalid, unable to resolve"}
	}
	log.Infof("Injecting chaos on {%v} hosts", finalHosts)
	return strings.Join(commaSeparatedIPs, ","), nil
}

// SetChaosTunables will set up a random value within a given range of values
// If the value is not provided in range it'll set up the initial provided value.
func SetChaosTunables(experimentsDetails *experimentTypes.ExperimentDetails) {
	experimentsDetails.NetworkPacketLossPercentage = common.ValidateRange(experimentsDetails.NetworkPacketLossPercentage)
	experimentsDetails.NetworkPacketCorruptionPercentage = common.ValidateRange(experimentsDetails.NetworkPacketCorruptionPercentage)
	experimentsDetails.NetworkPacketDuplicationPercentage = common.ValidateRange(experimentsDetails.NetworkPacketDuplicationPercentage)
	experimentsDetails.PodsAffectedPerc = common.ValidateRange(experimentsDetails.PodsAffectedPerc)
	experimentsDetails.Sequence = common.GetRandomSequence(experimentsDetails.Sequence)
}

// It checks if pod contains service mesh sidecar
func isServiceMeshEnabledForPod(pod apiv1.Pod) bool {
	for _, c := range pod.Spec.Containers {
		if common.SubStringExistsInSlice(c.Name, serviceMesh) {
			return true
		}
	}
	return false
}

func setDestIps(pod apiv1.Pod, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {
	var err error
	if isServiceMeshEnabledForPod(pod) {
		if destIpsSvcMesh == "" {
			destIpsSvcMesh, err = GetTargetIps(experimentsDetails.DestinationIPs, experimentsDetails.DestinationHosts, clients, true)
			if err != nil {
				return "false", err
			}
		}
		return "true", nil
	}
	if destIps == "" {
		destIps, err = GetTargetIps(experimentsDetails.DestinationIPs, experimentsDetails.DestinationHosts, clients, false)
		if err != nil {
			return "false", err
		}
	}
	return "false", nil
}

func filterPodsForNodes(targetPodList apiv1.PodList, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (map[string]*targetsDetails, error) {
	targets := make(map[string]*targetsDetails)
	targetContainer := experimentsDetails.TargetContainer

	for _, pod := range targetPodList.Items {
		serviceMesh, err := setDestIps(pod, experimentsDetails, clients)
		if err != nil {
			return targets, stacktrace.Propagate(err, "could not set destination ips")
		}

		if experimentsDetails.TargetContainer == "" {
			targetContainer = pod.Spec.Containers[0].Name
		}

		td := target{
			Name:            pod.Name,
			Namespace:       pod.Namespace,
			TargetContainer: targetContainer,
			ServiceMesh:     serviceMesh,
		}

		if targets[pod.Spec.NodeName] == nil {
			targets[pod.Spec.NodeName] = &targetsDetails{
				Target: []target{td},
			}
		} else {
			targets[pod.Spec.NodeName].Target = append(targets[pod.Spec.NodeName].Target, td)
		}
	}
	return targets, nil
}

func logExperimentFields(experimentsDetails *experimentTypes.ExperimentDetails) {
	switch experimentsDetails.NetworkChaosType {
	case "network-loss":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkPacketLossPercentage": experimentsDetails.NetworkPacketLossPercentage,
			"Sequence":                    experimentsDetails.Sequence,
			"PodsAffectedPerc":            experimentsDetails.PodsAffectedPerc,
			"Correlation":                 experimentsDetails.Correlation,
		})
	case "network-latency":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkLatency":   strconv.Itoa(experimentsDetails.NetworkLatency),
			"Jitter":           experimentsDetails.Jitter,
			"Sequence":         experimentsDetails.Sequence,
			"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
			"Correlation":      experimentsDetails.Correlation,
		})
	case "network-corruption":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkPacketCorruptionPercentage": experimentsDetails.NetworkPacketCorruptionPercentage,
			"Sequence":                          experimentsDetails.Sequence,
			"PodsAffectedPerc":                  experimentsDetails.PodsAffectedPerc,
			"Correlation":                       experimentsDetails.Correlation,
		})
	case "network-duplication":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkPacketDuplicationPercentage": experimentsDetails.NetworkPacketDuplicationPercentage,
			"Sequence":                           experimentsDetails.Sequence,
			"PodsAffectedPerc":                   experimentsDetails.PodsAffectedPerc,
			"Correlation":                        experimentsDetails.Correlation,
		})
	case "network-rate-limit":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkBandwidth": experimentsDetails.NetworkBandwidth,
			"Sequence":         experimentsDetails.Sequence,
			"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
			"Correlation":      experimentsDetails.Correlation,
		})
	}
}
