package lib

import (
	"strings"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-network-partition/types"
	networkv1 "k8s.io/api/networking/v1"
)

const (
	AllIPs string = "0.0.0.0/0"
)

// NetworkPolicy contains details about the network-policy
type NetworkPolicy struct {
	TargetPodLabels map[string]string
	PolicyType      []networkv1.PolicyType
	CIDR            string
	ExceptIPs       []string
}

// initialize creates an instance of network policy struct
func initialize() *NetworkPolicy {
	return &NetworkPolicy{}
}

// getNetworkPolicyDetails collects all the data required for network policy
func (np *NetworkPolicy) getNetworkPolicyDetails(experimentsDetails *experimentTypes.ExperimentDetails) error {
	_, err := np.setLabels(experimentsDetails.AppLabel).
		setPolicy(experimentsDetails.PolicyTypes).
		setCIDR().
		setExceptIPs(experimentsDetails)

	return err
}

// setLabels sets the target application label
func (np *NetworkPolicy) setLabels(appLabel string) *NetworkPolicy {
	labels := strings.Split(appLabel, "=")
	switch {
	case len(labels) == 2:
		np.TargetPodLabels = map[string]string{
			labels[0]: labels[1],
		}
	default:
		np.TargetPodLabels = map[string]string{
			labels[0]: "",
		}
	}
	return np
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

// setCIDR sets the CIDR for all the ips
func (np *NetworkPolicy) setCIDR() *NetworkPolicy {
	np.CIDR = AllIPs
	return np
}

// setExceptIPs sets all the destination ips
// for which traffic should be blocked
func (np *NetworkPolicy) setExceptIPs(experimentsDetails *experimentTypes.ExperimentDetails) (*NetworkPolicy, error) {
	// get all the target ips
	destinationIPs, err := network_chaos.GetTargetIps(experimentsDetails.DestinationIPs, experimentsDetails.DestinationHosts)
	if err != nil {
		return nil, err
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
		if !isPresent && !strings.Contains(ips[i], ":") {
			uniqueIps = append(uniqueIps, ips[i]+"/32")
		}
	}
	np.ExceptIPs = uniqueIps
	return np, nil
}
