package duplication

import (
	"strconv"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

var err error

//PodNetworkDuplicationChaos contains the steps to prepare and inject chaos
func PodNetworkDuplicationChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	args := "duplicate " + strconv.Itoa(experimentsDetails.NetworkPacketDuplicationPercentage)
	err = network_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
	if err != nil {
		return err
	}

	return nil
}
