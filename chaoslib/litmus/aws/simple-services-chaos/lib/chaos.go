package lib

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib/loss"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/aws/simple-services-chaos/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	networkExperimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
)

func injectChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ips, err := fetchIPs(experimentsDetails)
	if err != nil {
		return err
	}
	experiment := &networkExperimentTypes.ExperimentDetails{
		ExperimentName:     experimentsDetails.ExperimentName,
		ChaosDuration:      experimentsDetails.ChaosDuration,
		ChaosUID:           experimentsDetails.ChaosUID,
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
		Timeout:            experimentsDetails.Timeout,
		TCImage:            experimentsDetails.TCImage,
		Delay:              experimentsDetails.Delay,
		RampTime:           experimentsDetails.RampTime,
		ChaosPodName:       experimentsDetails.ChaosPodName,
		InstanceID:         experimentsDetails.InstanceID,
		Sequence:           experimentsDetails.Sequence,
		DestinationIPs:     strings.Join(ips, ","),
	}
	return loss.PodNetworkLossChaos(experiment, clients, resultDetails, eventsDetails, chaosDetails)
}

type CustomIP []string

func fetchIPs(experimentDetails *experimentTypes.ExperimentDetails) ([]string, error) {
	log.Infof("Fetching destination IPs for SNS on region %v...", experimentDetails.Region)
	ipsAsString := make(CustomIP, 0, experimentDetails.MinNumberOfIps)
	startTime := time.Now()
	for time.Since(startTime).Seconds() < float64(experimentDetails.TimeoutGatherMinNumberOfIps) {
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
	return nil, errors.Errorf("Unable to fetch at least %v IPs within %v seconds", experimentDetails.MinNumberOfIps, experimentDetails.TimeoutGatherMinNumberOfIps)
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
	if err := injectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
		return err
	}
	return nil
}
