package kafka

import (
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
)

// ClusterHealthCheck checks health of the kafka cluster
func ClusterHealthCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	// Checking Kafka pods status
	log.Info("[Status]: Verify that all the kafka pods are running")
	if err := status.CheckApplicationStatus(experimentsDetails.KafkaNamespace, experimentsDetails.KafkaLabel, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients); err != nil {
		return err
	}

	// Checking zookeeper pods status
	log.Info("[Status]: Verify that all the zookeeper pods are running")
	if err := status.CheckApplicationStatus(experimentsDetails.ZookeeperNamespace, experimentsDetails.ZookeeperLabel, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients); err != nil {
		return err
	}

	return nil
}

// DisplayKafkaBroker displays the kafka broker info
func DisplayKafkaBroker(experimentsDetails *experimentTypes.ExperimentDetails) {

	if experimentsDetails.KafkaBroker != "" {
		log.Infof("[Info]: Kafka broker pod for deletion is %v", experimentsDetails.KafkaBroker)
	} else {
		log.Info("[Info]: kafka broker will be selected randomly across the cluster")
	}
}
