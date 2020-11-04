package kafka

import (
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/pkg/errors"
)

// LaunchStreamDeriveLeader will derive broker pod leader
func LaunchStreamDeriveLeader(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	var err error
	LivenessTopicLeader, err := LivenessStream(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("liveness stream failed, err: %v", err)
	}
	experimentsDetails.TargetPod, err = SelectBroker(experimentsDetails, LivenessTopicLeader, clients)
	DisplayKafkaBroker(experimentsDetails)

	return nil
}

// SelectBroker will select leader broker as per the liveness topic (partition)
func SelectBroker(experimentsDetails *experimentTypes.ExperimentDetails, LivenessTopicLeader string, clients clients.ClientSets) (string, error) {
	if experimentsDetails.KafkaLivenessStream == "enabled" {
		return LivenessTopicLeader, nil
	}
	return "", nil
}
