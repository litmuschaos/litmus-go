package main

import (
	"flag"

	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
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
	podAutoscaler "github.com/litmuschaos/litmus-go/experiments/generic/pod-autoscaler/experiment"
	podCPUHog "github.com/litmuschaos/litmus-go/experiments/generic/pod-cpu-hog/experiment"
	podDelete "github.com/litmuschaos/litmus-go/experiments/generic/pod-delete/experiment"
	podIOStress "github.com/litmuschaos/litmus-go/experiments/generic/pod-io-stress/experiment"
	podMemoryHog "github.com/litmuschaos/litmus-go/experiments/generic/pod-memory-hog/experiment"
	podNetworkCorruption "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-corruption/experiment"
	podNetworkDuplication "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-duplication/experiment"
	podNetworkLatency "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-latency/experiment"
	podNetworkLoss "github.com/litmuschaos/litmus-go/experiments/generic/pod-network-loss/experiment"

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

	// parse the experiment name
	experimentName := flag.String("name", "pod-delete", "name of the chaos experiment")
	flag.Parse()

	log.Infof("Experiment Name: %v", *experimentName)

	// invoke the corresponding experiment based on the the (-name) flag
	switch *experimentName {
	case "container-kill":
		containerKill.ContainerKill()
	case "disk-fill":
		diskFill.DiskFill()
	case "kubelet-service-kill":
		kubeletServiceKill.KubeletServiceKill()
	case "node-cpu-hog":
		nodeCPUHog.NodeCPUHog()
	case "node-drain":
		nodeDrain.NodeDrain()
	case "node-io-stress":
		nodeIOStress.NodeIOStress()
	case "node-memory-hog":
		nodeMemoryHog.NodeMemoryHog()
	case "node-taint":
		nodeTaint.NodeTaint()
	case "pod-autoscaler":
		podAutoscaler.PodAutoscaler()
	case "pod-cpu-hog":
		podCPUHog.PodCPUHog()
	case "pod-delete":
		podDelete.PodDelete()
	case "pod-io-stress":
		podIOStress.PodIOStress()
	case "pod-memory-hog":
		podMemoryHog.PodMemoryHog()
	case "pod-network-corruption":
		podNetworkCorruption.PodNetworkCorruption()
	case "pod-network-duplication":
		podNetworkDuplication.PodNetworkDuplication()
	case "pod-network-latency":
		podNetworkLatency.PodNetworkLatency()
	case "pod-network-loss":
		podNetworkLoss.PodNetworkLoss()
	case "cassandra-pod-delete":
		cassandraPodDelete.CasssandraPodDelete()
	default:
		log.Fatalf("Unsupported -name %v, please provide the correct value of -name args", *experimentName)
	}

}
