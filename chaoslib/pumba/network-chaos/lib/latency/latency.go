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

	args, err := GetContainerArguments(experimentsDetails)
	if err != nil {
		return err
	}
	return network_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}

// GetContainerArguments derives the args for the pumba pod
func GetContainerArguments(experimentsDetails *experimentTypes.ExperimentDetails) ([]string, error) {
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
	args, err := network_chaos.AddTargetIpsArgs(experimentsDetails.DestinationIPs, experimentsDetails.DestinationHosts, args)
	if err != nil {
		return args, err
	}
	args = append(args, "delay", "--time", strconv.Itoa(experimentsDetails.NetworkLatency))

	return args, nil
}
