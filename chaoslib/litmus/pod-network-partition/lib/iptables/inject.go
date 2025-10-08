package iptables

import (
	"context"
	"fmt"
	"os/exec"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-network-partition/types"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

// InjectPartitionWithIptablesAndTC injects network partition using ipset, iptables, and tc in the target pod's netns
func InjectPartitionWithIptablesAndTC(
	ctx context.Context,
	experimentsDetails *experimentTypes.ExperimentDetails,
	clients clients.ClientSets,
	resultDetails *types.ResultDetails,
	eventsDetails *types.EventDetails,
	chaosDetails *types.ChaosDetails,
) error {
	// Example: get pod IPs, container ID, and run ipset/iptables/tc commands using nsenter/nsexec

	targetPodIP := experimentsDetails.TargetPodIP // You may need to fetch this from the pod object
	containerID := experimentsDetails.TargetContainerID // Set this up from ENV or pod status

	// 1. Create ipset for the target
	ipsetName := "chaos-partition"
	createIPSet := exec.Command("ipset", "create", ipsetName, "hash:net")
	_ = createIPSet.Run() // ignore error if exists

	// 2. Add target pod IP to ipset
	addToIPSet := exec.Command("ipset", "add", ipsetName, targetPodIP)
	if err := addToIPSet.Run(); err != nil {
		return fmt.Errorf("failed to add IP to ipset: %w", err)
	}

	// 3. Insert iptables rule to DROP traffic to target ipset
	iptablesCmd := exec.Command("iptables", "-I", "OUTPUT", "-m", "set", "--match-set", ipsetName, "dst", "-j", "DROP")
	if err := iptablesCmd.Run(); err != nil {
		return fmt.Errorf("failed to add iptables rule: %w", err)
	}

	// 4. Setup netem/tc if specified (e.g., delay, loss, etc)
	// Example: tc qdisc add dev eth0 root netem loss 100%%
	tcCmd := exec.Command("tc", "qdisc", "add", "dev", "eth0", "root", "netem", "loss", "100%%")
	_ = tcCmd.Run() // ignore error if already exists

	log.Infof("[Chaos]: Injected iptables+tc partition for %s", targetPodIP)

	// TODO: Use nsenter/nsexec to run commands in the pod's netns using container ID
	// TODO: Clean up rules after chaos duration

	return nil
}