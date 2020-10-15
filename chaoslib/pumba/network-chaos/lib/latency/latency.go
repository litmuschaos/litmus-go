package latency

import (
	"strconv"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/pumba/network-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

var err error

//PodNetworkLatencyChaos contains the steps to prepare and inject chaos
func PodNetworkLatencyChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	args := GetContainerArguments(experimentsDetails)
	err = network_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
	if err != nil {
		return err
	}

	return nil
}

// GetContainerArguments derives the args for the pumba pod
func GetContainerArguments(experimentsDetails *experimentTypes.ExperimentDetails) []string {
	baseArgs := []string{
		"netem",
		"--tc-image",
		experimentsDetails.TCImage,
		"--interface",
		experimentsDetails.NetworkInterface,
		"--duration",
		strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
	}

	args := baseArgs
	args = network_chaos.AddTargetIpsArgs(experimentsDetails.TargetIPs, args)
	args = network_chaos.AddTargetIpsArgs(network_chaos.GetIpsForTargetHosts(experimentsDetails.TargetHosts), args)
	args = append(args, "delay", "--time", strconv.Itoa(experimentsDetails.NetworkLatency))

	return args
}
