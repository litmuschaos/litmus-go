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
	nodeTaint "github.com/litmuschaos/litmus-go/experiments/generic/node-taint/experiment"
	ebsLoss "github.com/litmuschaos/litmus-go/experiments/generic/platform/aws/ebs-loss/experiment"
	podAutoscaler "github.com/litmuschaos/litmus-go/experiments/generic/pod-autoscaler/experiment"
	podCPUHog "github.com/litmuschaos/litmus-go/experiments/generic/pod-cpu-hog/experiment"
	podDelete "github.com/litmuschaos/litmus-go/experiments/generic/pod-delete/experiment"
	podIOStress "github.com/litmuschaos/litmus-go/experiments/generic/pod-io-stress/experiment"
	podMemoryHog "github.com/litmuschaos/litmus-go/experiments/generic/pod-memory-hog/experiment"
	podNetworkCorruption "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-corruption/experiment"
	podNetworkDuplication "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-duplication/experiment"
	podNetworkLatency "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-latency/experiment"
	podNetworkLoss "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-loss/experiment"
	kafkaBrokerPodFailure "github.com/litmuschaos/litmus-go/experiments/kafka/kafka-broker-pod-failure/experiment"

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
		log.Fatalf("Unable to Get the kubeconfig, err: %v", err)
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
	case "ebs-loss":
		ebsLoss.EBSLoss(clients)
	default:
		log.Fatalf("Unsupported -name %v, please provide the correct value of -name args", *experimentName)
	}
}
