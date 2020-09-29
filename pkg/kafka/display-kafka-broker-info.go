package kafka

import (
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
)

// DisplayKafkaBroker will display the kafka broker info
func DisplayKafkaBroker(experimentsDetails *experimentTypes.ExperimentDetails) {

	if experimentsDetails.ChaoslibDetail.TargetPod != "" {
		log.Infof("Kafka Broker is %v", experimentsDetails.ChaoslibDetail.TargetPod)
	} else if experimentsDetails.ChaoslibDetail.TargetPod == "" {
		log.Info("kafka broker will be selected randomly across the cluster")
	}
}
