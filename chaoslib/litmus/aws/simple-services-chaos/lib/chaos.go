package lib

import (
	"fmt"
	"net"
	"strings"

	"github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib/loss"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/aws/simple-services-chaos/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	networkExperimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
)

func injectChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ips, err := fetchIPs(experimentsDetails)
	if err != nil {
		return err
	}
	experiment := &networkExperimentTypes.ExperimentDetails{
		ExperimentName:     experimentsDetails.ExperimentName,
		ChaosDuration:      experimentsDetails.ChaosDuration,
		ChaosNamespace:     experimentsDetails.ChaosNamespace,
		TargetPods:         experimentsDetails.TargetPods,
		TargetContainer:    experimentsDetails.TargetContainer,
		AppKind:            experimentsDetails.AppKind,
		AppNS:              experimentsDetails.AppNS,
		AppLabel:           experimentsDetails.AppLabel,
		PodsAffectedPerc:   experimentsDetails.PodsAffectedPerc,
		ChaosLib:           experimentsDetails.ChaosLib,
		LIBImage:           experimentsDetails.LIBImage,
		LIBImagePullPolicy: experimentsDetails.LIBImagePullPolicy,
		SocketPath:         experimentsDetails.SocketPath,
		TCImage:            experimentsDetails.TCImage,
		Timeout:            experimentsDetails.Timeout,
		Delay:              experimentsDetails.Delay,
		RampTime:           experimentsDetails.RampTime,
		ChaosPodName:       experimentsDetails.ChaosPodName,
		InstanceID:         experimentsDetails.InstanceID,
		DestinationIPs:     strings.Join(ips, ","),
	}
	return loss.PodNetworkLossChaos(experiment, clients, resultDetails, eventsDetails, chaosDetails)
}

type CustomIP []string

func fetchIPs(experimentDetails *experimentTypes.ExperimentDetails) ([]string, error) {
	log.Infof("Fetching destination IPs for SNS on region %v...", experimentDetails.Region)
	ipsAsString := make(CustomIP, 0, experimentDetails.MinNumberOfIps)
	for {
		ips, err := net.LookupIP(fmt.Sprintf("%v.%v.amazonaws.com", experimentDetails.AwsService, experimentDetails.Region))
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !ipsAsString.Contains(ip.To4().String()) {
				log.Infof("New IP:%v", ip.To4().String())
				ipsAsString = append(ipsAsString, ip.To4().String())
			}
		}

		if len(ipsAsString) >= experimentDetails.MinNumberOfIps {
			log.Info("Reached the minimum number of IPs")
			return ipsAsString, nil
		}
	}
}

func (ips CustomIP) Contains(value string) bool {
	for _, entry := range ips {
		if entry == value {
			return true
		}
	}
	return false
}

func PrepareChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	if err := injectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
		return err
	}
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}
