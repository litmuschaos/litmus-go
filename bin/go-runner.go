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

	cassandraPodDelete "github.com/litmuschaos/litmus-go/experiments/cassandra/pod-delete/experiment"
	containerKill "github.com/litmuschaos/litmus-go/experiments/generic/container-kill/experiment"
	diskFill "github.com/litmuschaos/litmus-go/experiments/generic/disk-fill/experiment"
	kubeletServiceKill "github.com/litmuschaos/litmus-go/experiments/generic/kubelet-service-kill/experiment"
	nodeCPUHog "github.com/litmuschaos/litmus-go/experiments/generic/node-cpu-hog/experiment"
	nodeDrain "github.com/litmuschaos/litmus-go/experiments/generic/node-drain/experiment"
	nodeIOStress "github.com/litmuschaos/litmus-go/experiments/generic/node-io-stress/experiment"
	nodeMemoryHog "github.com/litmuschaos/litmus-go/experiments/generic/node-memory-hog/experiment"
	nodeRestart "github.com/litmuschaos/litmus-go/experiments/generic/node-restart/experiment"
	nodeTaint "github.com/litmuschaos/litmus-go/experiments/generic/node-taint/experiment"
	podAutoscaler "github.com/litmuschaos/litmus-go/experiments/generic/pod-autoscaler/experiment"
	podCPUHog "github.com/litmuschaos/litmus-go/experiments/generic/pod-cpu-hog/experiment"
	podDelete "github.com/litmuschaos/litmus-go/experiments/generic/pod-delete/experiment"
	podDNSError "github.com/litmuschaos/litmus-go/experiments/generic/pod-dns-error/experiment"
	podDNSSpoof "github.com/litmuschaos/litmus-go/experiments/generic/pod-dns-spoof/experiment"
	podIOStress "github.com/litmuschaos/litmus-go/experiments/generic/pod-io-stress/experiment"
	podMemoryHog "github.com/litmuschaos/litmus-go/experiments/generic/pod-memory-hog/experiment"
	podNetworkCorruption "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-corruption/experiment"
	podNetworkDuplication "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-duplication/experiment"
	podNetworkLatency "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-latency/experiment"
	podNetworkLoss "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-loss/experiment"
	kafkaBrokerPodFailure "github.com/litmuschaos/litmus-go/experiments/kafka/kafka-broker-pod-failure/experiment"
	ebsLoss "github.com/litmuschaos/litmus-go/experiments/kube-aws/ebs-loss/experiment"
	ec2TerminateByID "github.com/litmuschaos/litmus-go/experiments/kube-aws/ec2-terminate-by-id/experiment"
	ec2TerminateByTag "github.com/litmuschaos/litmus-go/experiments/kube-aws/ec2-terminate-by-tag/experiment"
	vmpoweroff "github.com/litmuschaos/litmus-go/experiments/vmware/vm-poweroff/experiment"

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
	case "kafka-broker-pod-failure":
		kafkaBrokerPodFailure.KafkaBrokerPodFailure(clients)
	case "kubelet-service-kill":
		kubeletServiceKill.KubeletServiceKill(clients)
	case "node-cpu-hog":
		nodeCPUHog.NodeCPUHog(clients)
	case "node-drain":
		nodeDrain.NodeDrain(clients)
	case "node-io-stress":
		nodeIOStress.NodeIOStress(clients)
	case "node-memory-hog":
		nodeMemoryHog.NodeMemoryHog(clients)
	case "node-taint":
		nodeTaint.NodeTaint(clients)
	case "pod-autoscaler":
		podAutoscaler.PodAutoscaler(clients)
	case "pod-cpu-hog":
		podCPUHog.PodCPUHog(clients)
	case "pod-delete":
		podDelete.PodDelete(clients)
	case "pod-io-stress":
		podIOStress.PodIOStress(clients)
	case "pod-memory-hog":
		podMemoryHog.PodMemoryHog(clients)
	case "pod-network-corruption":
		podNetworkCorruption.PodNetworkCorruption(clients)
	case "pod-network-duplication":
		podNetworkDuplication.PodNetworkDuplication(clients)
	case "pod-network-latency":
		podNetworkLatency.PodNetworkLatency(clients)
	case "pod-network-loss":
		podNetworkLoss.PodNetworkLoss(clients)
	case "cassandra-pod-delete":
		cassandraPodDelete.CasssandraPodDelete(clients)
	case "ec2-terminate-by-id":
		ec2TerminateByID.EC2TerminateByID(clients)
	case "ec2-terminate-by-tag":
		ec2TerminateByTag.EC2TerminateByTag(clients)
	case "ebs-loss":
		ebsLoss.EBSLoss(clients)
	case "node-restart":
		nodeRestart.NodeRestart(clients)
	case "pod-dns-error":
		podDNSError.PodDNSError(clients)
	case "pod-dns-spoof":
		podDNSSpoof.PodDNSSpoof(clients)
	case "vm-poweroff":
		vmpoweroff.VMPoweroff(clients)

	default:
		log.Errorf("Unsupported -name %v, please provide the correct value of -name args", *experimentName)
		return
	}
}
