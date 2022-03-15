package main

import (
	"flag"
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"

	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/openstack"

	containerKill "github.com/litmuschaos/litmus-go/experiments/generic/container-kill/experiment"
	diskFill "github.com/litmuschaos/litmus-go/experiments/generic/disk-fill/experiment"
	podDelete "github.com/litmuschaos/litmus-go/experiments/generic/pod-delete/experiment"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/sirupsen/logrus"
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

	clients := clients.ClientSets{}

	// parse the experiment name
	experimentName := flag.String("name", "pod-delete", "name of the chaos experiment")

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Errorf("Unable to Get the kubeconfig, err: %v", err)
		return
	}

	log.Infof("Experiment Name: %v", *experimentName)

	// invoke the corresponding experiment based on the the (-name) flag
	switch *experimentName {
	case "container-kill":
		containerKill.ContainerKill(clients)
	case "disk-fill":
		diskFill.DiskFill(clients)
	// case "kafka-broker-pod-failure":
	// 	kafkaBrokerPodFailure.KafkaBrokerPodFailure(clients)
	// case "kubelet-service-kill":
	// 	kubeletServiceKill.KubeletServiceKill(clients)
	// case "docker-service-kill":
	// 	dockerServiceKill.DockerServiceKill(clients)
	// case "node-cpu-hog":
	// 	nodeCPUHog.NodeCPUHog(clients)
	// case "node-drain":
	// 	nodeDrain.NodeDrain(clients)
	// case "node-io-stress":
	// 	nodeIOStress.NodeIOStress(clients)
	// case "node-memory-hog":
	// 	nodeMemoryHog.NodeMemoryHog(clients)
	// case "node-taint":
	// 	nodeTaint.NodeTaint(clients)
	// case "pod-autoscaler":
	// 	podAutoscaler.PodAutoscaler(clients)
	// case "pod-cpu-hog-exec":
	// 	podCPUHogExec.PodCPUHogExec(clients)
	case "pod-delete":
		podDelete.PodDelete(clients)
	// case "pod-io-stress":
	// 	podIOStress.PodIOStress(clients)
	// case "pod-memory-hog-exec":
	// 	podMemoryHogExec.PodMemoryHogExec(clients)
	// case "pod-network-corruption":
	// 	podNetworkCorruption.PodNetworkCorruption(clients)
	// case "pod-network-duplication":
	// 	podNetworkDuplication.PodNetworkDuplication(clients)
	// case "pod-network-latency":
	// 	podNetworkLatency.PodNetworkLatency(clients)
	// case "pod-network-loss":
	// 	podNetworkLoss.PodNetworkLoss(clients)
	// case "pod-network-partition":
	// 	podNetworkPartition.PodNetworkPartition(clients)
	// case "pod-memory-hog":
	// 	podMemoryHog.PodMemoryHog(clients)
	// case "pod-cpu-hog":
	// 	podCPUHog.PodCPUHog(clients)
	// case "cassandra-pod-delete":
	// 	cassandraPodDelete.CasssandraPodDelete(clients)
	// case "aws-ssm-chaos-by-id":
	// 	awsSSMChaosByID.AWSSSMChaosByID(clients)
	// case "aws-ssm-chaos-by-tag":
	// 	awsSSMChaosByTag.AWSSSMChaosByTag(clients)
	// case "ec2-terminate-by-id":
	// 	ec2TerminateByID.EC2TerminateByID(clients)
	// case "ec2-terminate-by-tag":
	// 	ec2TerminateByTag.EC2TerminateByTag(clients)
	// case "ebs-loss-by-id":
	// 	ebsLossByID.EBSLossByID(clients)
	// case "ebs-loss-by-tag":
	// 	ebsLossByTag.EBSLossByTag(clients)
	// case "node-restart":
	// 	nodeRestart.NodeRestart(clients)
	// case "pod-dns-error":
	// 	podDNSError.PodDNSError(clients)
	// case "pod-dns-spoof":
	// 	podDNSSpoof.PodDNSSpoof(clients)
	// case "vm-poweroff":
	// 	vmpoweroff.VMPoweroff(clients)
	// case "azure-instance-stop":
	// 	azureInstanceStop.AzureInstanceStop(clients)
	// case "azure-disk-loss":
	// 	azureDiskLoss.AzureDiskLoss(clients)
	// case "gcp-vm-disk-loss":
	// 	gcpVMDiskLoss.VMDiskLoss(clients)
	// case "pod-fio-stress":
	// 	podFioStress.PodFioStress(clients)
	// case "gcp-vm-instance-stop":
	// 	gcpVMInstanceStop.VMInstanceStop(clients)
	// case "redfish-node-restart":
	// 	redfishNodeRestart.NodeRestart(clients)

	default:
		log.Errorf("Unsupported -name %v, please provide the correct value of -name args", *experimentName)
		return
	}
}
