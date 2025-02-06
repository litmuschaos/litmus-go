package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"

	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/openstack"

	awsSSMChaosByID "github.com/litmuschaos/litmus-go/experiments/aws-ssm/aws-ssm-chaos-by-id/experiment"
	awsSSMChaosByTag "github.com/litmuschaos/litmus-go/experiments/aws-ssm/aws-ssm-chaos-by-tag/experiment"
	azureDiskLoss "github.com/litmuschaos/litmus-go/experiments/azure/azure-disk-loss/experiment"
	azureInstanceStop "github.com/litmuschaos/litmus-go/experiments/azure/instance-stop/experiment"
	redfishNodeRestart "github.com/litmuschaos/litmus-go/experiments/baremetal/redfish-node-restart/experiment"
	cassandraPodDelete "github.com/litmuschaos/litmus-go/experiments/cassandra/pod-delete/experiment"
	gcpVMDiskLossByLabel "github.com/litmuschaos/litmus-go/experiments/gcp/gcp-vm-disk-loss-by-label/experiment"
	gcpVMDiskLoss "github.com/litmuschaos/litmus-go/experiments/gcp/gcp-vm-disk-loss/experiment"
	gcpVMInstanceStopByLabel "github.com/litmuschaos/litmus-go/experiments/gcp/gcp-vm-instance-stop-by-label/experiment"
	gcpVMInstanceStop "github.com/litmuschaos/litmus-go/experiments/gcp/gcp-vm-instance-stop/experiment"
	containerKill "github.com/litmuschaos/litmus-go/experiments/generic/container-kill/experiment"
	diskFill "github.com/litmuschaos/litmus-go/experiments/generic/disk-fill/experiment"
	dockerServiceKill "github.com/litmuschaos/litmus-go/experiments/generic/docker-service-kill/experiment"
	kubeletServiceKill "github.com/litmuschaos/litmus-go/experiments/generic/kubelet-service-kill/experiment"
	nodeCPUHog "github.com/litmuschaos/litmus-go/experiments/generic/node-cpu-hog/experiment"
	nodeDrain "github.com/litmuschaos/litmus-go/experiments/generic/node-drain/experiment"
	nodeIOStress "github.com/litmuschaos/litmus-go/experiments/generic/node-io-stress/experiment"
	nodeMemoryHog "github.com/litmuschaos/litmus-go/experiments/generic/node-memory-hog/experiment"
	nodeRestart "github.com/litmuschaos/litmus-go/experiments/generic/node-restart/experiment"
	nodeTaint "github.com/litmuschaos/litmus-go/experiments/generic/node-taint/experiment"
	podAutoscaler "github.com/litmuschaos/litmus-go/experiments/generic/pod-autoscaler/experiment"
	podCPUHogExec "github.com/litmuschaos/litmus-go/experiments/generic/pod-cpu-hog-exec/experiment"
	podCPUHog "github.com/litmuschaos/litmus-go/experiments/generic/pod-cpu-hog/experiment"
	podDelete "github.com/litmuschaos/litmus-go/experiments/generic/pod-delete/experiment"
	podDNSError "github.com/litmuschaos/litmus-go/experiments/generic/pod-dns-error/experiment"
	podDNSSpoof "github.com/litmuschaos/litmus-go/experiments/generic/pod-dns-spoof/experiment"
	podFioStress "github.com/litmuschaos/litmus-go/experiments/generic/pod-fio-stress/experiment"
	podHttpLatency "github.com/litmuschaos/litmus-go/experiments/generic/pod-http-latency/experiment"
	podHttpModifyBody "github.com/litmuschaos/litmus-go/experiments/generic/pod-http-modify-body/experiment"
	podHttpModifyHeader "github.com/litmuschaos/litmus-go/experiments/generic/pod-http-modify-header/experiment"
	podHttpResetPeer "github.com/litmuschaos/litmus-go/experiments/generic/pod-http-reset-peer/experiment"
	podHttpStatusCode "github.com/litmuschaos/litmus-go/experiments/generic/pod-http-status-code/experiment"
	podIOStress "github.com/litmuschaos/litmus-go/experiments/generic/pod-io-stress/experiment"
	podMemoryHogExec "github.com/litmuschaos/litmus-go/experiments/generic/pod-memory-hog-exec/experiment"
	podMemoryHog "github.com/litmuschaos/litmus-go/experiments/generic/pod-memory-hog/experiment"
	podNetworkCorruption "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-corruption/experiment"
	podNetworkDuplication "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-duplication/experiment"
	podNetworkLatency "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-latency/experiment"
	podNetworkLoss "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-loss/experiment"
	podNetworkPartition "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-partition/experiment"
	kafkaBrokerPodFailure "github.com/litmuschaos/litmus-go/experiments/kafka/kafka-broker-pod-failure/experiment"
	ebsLossByID "github.com/litmuschaos/litmus-go/experiments/kube-aws/ebs-loss-by-id/experiment"
	ebsLossByTag "github.com/litmuschaos/litmus-go/experiments/kube-aws/ebs-loss-by-tag/experiment"
	ec2TerminateByID "github.com/litmuschaos/litmus-go/experiments/kube-aws/ec2-terminate-by-id/experiment"
	ec2TerminateByTag "github.com/litmuschaos/litmus-go/experiments/kube-aws/ec2-terminate-by-tag/experiment"
	k6Loadgen "github.com/litmuschaos/litmus-go/experiments/load/k6-loadgen/experiment"
	springBootFaults "github.com/litmuschaos/litmus-go/experiments/spring-boot/spring-boot-faults/experiment"
	vmpoweroff "github.com/litmuschaos/litmus-go/experiments/vmware/vm-poweroff/experiment"
	cli "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:          true,
		DisableSorting:         true,
		DisableLevelTruncation: true,
	})
}

func main() {
	initCtx := context.Background()

	// Set up Observability.
	if otelExporterEndpoint := os.Getenv(telemetry.OTELExporterOTLPEndpoint); otelExporterEndpoint != "" {
		shutdown, err := telemetry.InitOTelSDK(initCtx, true, otelExporterEndpoint)
		if err != nil {
			log.Errorf("Failed to initialize OTel SDK: %v", err)
			return
		}
		defer func() {
			err = errors.Join(err, shutdown(initCtx))
		}()
		initCtx = telemetry.GetTraceParentContext()
	}

	clients := cli.ClientSets{}

	ctx, span := otel.Tracer(telemetry.TracerName).Start(initCtx, "ExecuteExperiment")
	defer span.End()

	// parse the experiment name
	experimentName := flag.String("name", "pod-delete", "name of the chaos experiment")

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Errorf("Unable to Get the kubeconfig, err: %v", err)
		span.SetStatus(codes.Error, "Unable to Get the kubeconfig")
		span.RecordError(err)
		return
	}

	log.Infof("Experiment Name: %v", *experimentName)

	// invoke the corresponding experiment based on the (-name) flag
	switch *experimentName {
	case "container-kill":
		containerKill.ContainerKill(ctx, clients)
	case "disk-fill":
		diskFill.DiskFill(ctx, clients)
	case "kafka-broker-pod-failure":
		kafkaBrokerPodFailure.KafkaBrokerPodFailure(ctx, clients)
	case "kubelet-service-kill":
		kubeletServiceKill.KubeletServiceKill(ctx, clients)
	case "docker-service-kill":
		dockerServiceKill.DockerServiceKill(ctx, clients)
	case "node-cpu-hog":
		nodeCPUHog.NodeCPUHog(ctx, clients)
	case "node-drain":
		nodeDrain.NodeDrain(ctx, clients)
	case "node-io-stress":
		nodeIOStress.NodeIOStress(ctx, clients)
	case "node-memory-hog":
		nodeMemoryHog.NodeMemoryHog(ctx, clients)
	case "node-taint":
		nodeTaint.NodeTaint(ctx, clients)
	case "pod-autoscaler":
		podAutoscaler.PodAutoscaler(ctx, clients)
	case "pod-cpu-hog-exec":
		podCPUHogExec.PodCPUHogExec(ctx, clients)
	case "pod-delete":
		podDelete.PodDelete(ctx, clients)
	case "pod-io-stress":
		podIOStress.PodIOStress(ctx, clients)
	case "pod-memory-hog-exec":
		podMemoryHogExec.PodMemoryHogExec(ctx, clients)
	case "pod-network-corruption":
		podNetworkCorruption.PodNetworkCorruption(ctx, clients)
	case "pod-network-duplication":
		podNetworkDuplication.PodNetworkDuplication(ctx, clients)
	case "pod-network-latency":
		podNetworkLatency.PodNetworkLatency(ctx, clients)
	case "pod-network-loss":
		podNetworkLoss.PodNetworkLoss(ctx, clients)
	case "pod-network-partition":
		podNetworkPartition.PodNetworkPartition(ctx, clients)
	case "pod-memory-hog":
		podMemoryHog.PodMemoryHog(ctx, clients)
	case "pod-cpu-hog":
		podCPUHog.PodCPUHog(ctx, clients)
	case "cassandra-pod-delete":
		cassandraPodDelete.CasssandraPodDelete(ctx, clients)
	case "aws-ssm-chaos-by-id":
		awsSSMChaosByID.AWSSSMChaosByID(ctx, clients)
	case "aws-ssm-chaos-by-tag":
		awsSSMChaosByTag.AWSSSMChaosByTag(ctx, clients)
	case "ec2-terminate-by-id":
		ec2TerminateByID.EC2TerminateByID(ctx, clients)
	case "ec2-terminate-by-tag":
		ec2TerminateByTag.EC2TerminateByTag(ctx, clients)
	case "ebs-loss-by-id":
		ebsLossByID.EBSLossByID(ctx, clients)
	case "ebs-loss-by-tag":
		ebsLossByTag.EBSLossByTag(ctx, clients)
	case "node-restart":
		nodeRestart.NodeRestart(ctx, clients)
	case "pod-dns-error":
		podDNSError.PodDNSError(ctx, clients)
	case "pod-dns-spoof":
		podDNSSpoof.PodDNSSpoof(ctx, clients)
	case "pod-http-latency":
		podHttpLatency.PodHttpLatency(ctx, clients)
	case "pod-http-status-code":
		podHttpStatusCode.PodHttpStatusCode(ctx, clients)
	case "pod-http-modify-header":
		podHttpModifyHeader.PodHttpModifyHeader(ctx, clients)
	case "pod-http-modify-body":
		podHttpModifyBody.PodHttpModifyBody(ctx, clients)
	case "pod-http-reset-peer":
		podHttpResetPeer.PodHttpResetPeer(ctx, clients)
	case "vm-poweroff":
		vmpoweroff.VMPoweroff(ctx, clients)
	case "azure-instance-stop":
		azureInstanceStop.AzureInstanceStop(ctx, clients)
	case "azure-disk-loss":
		azureDiskLoss.AzureDiskLoss(ctx, clients)
	case "gcp-vm-disk-loss":
		gcpVMDiskLoss.VMDiskLoss(ctx, clients)
	case "pod-fio-stress":
		podFioStress.PodFioStress(ctx, clients)
	case "gcp-vm-instance-stop":
		gcpVMInstanceStop.VMInstanceStop(ctx, clients)
	case "redfish-node-restart":
		redfishNodeRestart.NodeRestart(ctx, clients)
	case "gcp-vm-instance-stop-by-label":
		gcpVMInstanceStopByLabel.GCPVMInstanceStopByLabel(ctx, clients)
	case "gcp-vm-disk-loss-by-label":
		gcpVMDiskLossByLabel.GCPVMDiskLossByLabel(ctx, clients)
	case "spring-boot-cpu-stress", "spring-boot-memory-stress", "spring-boot-exceptions", "spring-boot-app-kill", "spring-boot-faults", "spring-boot-latency":
		springBootFaults.Experiment(ctx, clients, *experimentName)
	case "k6-loadgen":
		k6Loadgen.Experiment(ctx, clients)
	default:
		log.Errorf("Unsupported -name %v, please provide the correct value of -name args", *experimentName)
		span.SetStatus(codes.Error, fmt.Sprintf("Unsupported -name %v", *experimentName))
		return
	}
}
