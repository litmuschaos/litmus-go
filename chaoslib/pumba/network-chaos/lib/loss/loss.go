package loss

import (
	"strconv"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/pumba/network-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

//PodNetworkLossChaos contains the steps to prepare and inject chaos
func PodNetworkLossChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	args, err := getContainerArguments(experimentsDetails)
	if err != nil {
		return err
	}
	return network_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}

// getContainerArguments derives the args for the pumba pod
func getContainerArguments(experimentsDetails *experimentTypes.ExperimentDetails) ([]string, error) {
	baseArgs := []string{
		"pumba",
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
	args = append(args, "loss", "--percent", strconv.Itoa(experimentsDetails.NetworkPacketLossPercentage))

	return args, nil
}
