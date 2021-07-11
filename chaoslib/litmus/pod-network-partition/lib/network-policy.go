package lib

import (
	"strings"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-network-partition/types"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	AllIPs string = "0.0.0.0/0"
)

// NetworkPolicy contains details about the network-policy
type NetworkPolicy struct {
	TargetPodLabels   map[string]string
	PolicyType        []networkv1.PolicyType
	Egress            []networkv1.NetworkPolicyEgressRule
	Ingress           []networkv1.NetworkPolicyIngressRule
	ExceptIPs         []string
	NamespaceSelector map[string]string
	PodSelector       map[string]string
	Ports             []networkv1.NetworkPolicyPort
}

// Port contains the port details
type Port struct {
	TCP  []int32 `json:"tcp"`
	UDP  []int32 `json:"udp"`
	SCTP []int32 `json:"sctp"`
}

// initialize creates an instance of network policy struct
func initialize() *NetworkPolicy {
	return &NetworkPolicy{}
}

// getNetworkPolicyDetails collects all the data required for network policy
func (np *NetworkPolicy) getNetworkPolicyDetails(experimentsDetails *experimentTypes.ExperimentDetails) error {
	np.setLabels(experimentsDetails.AppLabel).
		setPolicy(experimentsDetails.PolicyTypes).
		setPodSelector(experimentsDetails.PodSelector).
		setNamespaceSelector(experimentsDetails.NamespaceSelector)

	// sets the ports for the traffic control
	if err := np.setPort(experimentsDetails.PORTS); err != nil {
		return err
	}

	// sets the destination ips for which the traffic should be blocked
	if err := np.setExceptIPs(experimentsDetails); err != nil {
		return err
	}

	// sets the egress traffic rules
	if strings.ToLower(experimentsDetails.PolicyTypes) == "egress" || strings.ToLower(experimentsDetails.PolicyTypes) == "all" {
		np.setEgressRules()
	}

	// sets the ingress traffic rules
	if strings.ToLower(experimentsDetails.PolicyTypes) == "ingress" || strings.ToLower(experimentsDetails.PolicyTypes) == "all" {
		np.setIngressRules()
	}

	return nil
}

// setLabels sets the target application label
func (np *NetworkPolicy) setLabels(appLabel string) *NetworkPolicy {
	key, value := getKeyValue(appLabel)
	if key != "" || value != "" {
		np.TargetPodLabels = map[string]string{
			key: value,
		}
	}
	return np
}

// getKeyValue returns the key & value from the label
func getKeyValue(label string) (string, string) {
	labels := strings.Split(label, "=")
	switch {
	case len(labels) == 2:
		return labels[0], labels[1]
	default:
		return labels[0], ""
	}
}

// setPolicy sets the network policy types
func (np *NetworkPolicy) setPolicy(policy string) *NetworkPolicy {
	switch strings.ToLower(policy) {
	case "ingress":
		np.PolicyType = []networkv1.PolicyType{networkv1.PolicyTypeIngress}
	case "egress":
		np.PolicyType = []networkv1.PolicyType{networkv1.PolicyTypeEgress}
	default:
		np.PolicyType = []networkv1.PolicyType{networkv1.PolicyTypeEgress, networkv1.PolicyTypeIngress}
	}
	return np
}

// setPodSelector sets the pod labels selector
func (np *NetworkPolicy) setPodSelector(podLabel string) *NetworkPolicy {
	podSelector := map[string]string{}
	labels := strings.Split(podLabel, ",")
	for i := range labels {
		key, value := getKeyValue(labels[i])
		if key != "" || value != "" {
			podSelector[key] = value
		}
	}
	np.PodSelector = podSelector
	return np
}

// setNamespaceSelector sets the namespace labels selector
func (np *NetworkPolicy) setNamespaceSelector(nsLabel string) *NetworkPolicy {
	nsSelector := map[string]string{}
	labels := strings.Split(nsLabel, ",")
	for i := range labels {
		key, value := getKeyValue(labels[i])
		if key != "" || value != "" {
			nsSelector[key] = value
		}
	}
	np.NamespaceSelector = nsSelector
	return np
}

// setPort sets all the protocols and ports
func (np *NetworkPolicy) setPort(p string) error {
	ports := []networkv1.NetworkPolicyPort{}
	var port Port
	// unmarshal the protocols and ports from the env
	if err := yaml.Unmarshal([]byte(strings.TrimSpace(parseCommand(p))), &port); err != nil {
		return errors.Errorf("Unable to unmarshal, err: %v", err)
	}

	// sets all the tcp ports
	for _, p := range port.TCP {
		ports = append(ports, getPort(p, corev1.ProtocolTCP))
	}

	// sets all the udp ports
	for _, p := range port.UDP {
		ports = append(ports, getPort(p, corev1.ProtocolUDP))
	}

	// sets all the sctp ports
	for _, p := range port.SCTP {
		ports = append(ports, getPort(p, corev1.ProtocolSCTP))
	}

	np.Ports = ports
	return nil
}

// getPort return the port details
func getPort(port int32, protocol corev1.Protocol) networkv1.NetworkPolicyPort {
	networkPorts := networkv1.NetworkPolicyPort{
		Protocol: &protocol,
		Port: &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: port,
		},
	}
	return networkPorts
}

// setExceptIPs sets all the destination ips
// for which traffic should be blocked
func (np *NetworkPolicy) setExceptIPs(experimentsDetails *experimentTypes.ExperimentDetails) error {
	// get all the target ips
	destinationIPs, err := network_chaos.GetTargetIps(experimentsDetails.DestinationIPs, experimentsDetails.DestinationHosts)
	if err != nil {
		return err
	}

	ips := strings.Split(destinationIPs, ",")
	var uniqueIps []string
	// removing all the duplicates and ipv6 ips from the list, if any
	for i := range ips {
		isPresent := false
		for j := range uniqueIps {
			if ips[i] == uniqueIps[j] {
				isPresent = true
			}
		}
		if ips[i] != "" && !isPresent && !strings.Contains(ips[i], ":") {
			uniqueIps = append(uniqueIps, ips[i]+"/32")
		}
	}
	np.ExceptIPs = uniqueIps
	return nil
}

// setIngressRules sets the ingress traffic rules
func (np *NetworkPolicy) setIngressRules() *NetworkPolicy {

	if len(np.getPeers()) != 0 || len(np.Ports) != 0 {
		np.Ingress = []networkv1.NetworkPolicyIngressRule{
			{
				From:  np.getPeers(),
				Ports: np.Ports,
			},
		}
	}
	return np
}

// setEgressRules sets the egress traffic rules
func (np *NetworkPolicy) setEgressRules() *NetworkPolicy {

	if len(np.getPeers()) != 0 || len(np.Ports) != 0 {
		np.Egress = []networkv1.NetworkPolicyEgressRule{
			{
				To:    np.getPeers(),
				Ports: np.Ports,
			},
		}
	}
	return np
}

// getPeers return the peer's ips, namespace selectors, and pod selectors
func (np *NetworkPolicy) getPeers() []networkv1.NetworkPolicyPeer {
	var peers []networkv1.NetworkPolicyPeer

	// sets the namespace selectors
	if np.NamespaceSelector != nil && len(np.NamespaceSelector) != 0 {
		peers = append(peers, np.getNamespaceSelector())
	}

	// sets the pod selectors
	if np.PodSelector != nil && len(np.PodSelector) != 0 {
		peers = append(peers, np.getPodSelector())
	}

	// sets the ipblocks
	if np.ExceptIPs != nil && len(np.ExceptIPs) != 0 {
		peers = append(peers, np.getIPBlocks())
	}

	return peers
}

// getNamespaceSelector builds the namespace selector
func (np *NetworkPolicy) getNamespaceSelector() networkv1.NetworkPolicyPeer {
	nsSelector := networkv1.NetworkPolicyPeer{
		NamespaceSelector: &v1.LabelSelector{
			MatchLabels: np.NamespaceSelector,
		},
	}
	return nsSelector
}

// getPodSelector builds the pod selectors
func (np *NetworkPolicy) getPodSelector() networkv1.NetworkPolicyPeer {
	podSelector := networkv1.NetworkPolicyPeer{
		PodSelector: &v1.LabelSelector{
			MatchLabels: np.PodSelector,
		},
	}
	return podSelector
}

// getIPBlocks builds the ipblocks
func (np *NetworkPolicy) getIPBlocks() networkv1.NetworkPolicyPeer {
	ipBlocks := networkv1.NetworkPolicyPeer{
		IPBlock: &networkv1.IPBlock{
			CIDR:   AllIPs,
			Except: np.ExceptIPs,
		},
	}
	return ipBlocks
}

// parseCommand parse the protocols and ports
func parseCommand(command string) string {
	final := ""
	c := strings.Split(command, ", ")
	for i := range c {
		final = final + strings.TrimSpace(c[i]) + "\n"
	}
	return final
}
