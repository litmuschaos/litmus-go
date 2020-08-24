package kafka

import (
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
)

// DisplayKafkaBrocker will display the kafka broker info
func DisplayKafkaBrocker(experimentsDetails *experimentTypes.ExperimentDetails) {

	if experimentsDetails.KafkaBroker != "" {
		log.Infof("Kafka Broker is %v", experimentsDetails.KafkaBroker)
	} else if experimentsDetails.KafkaBroker == "" {
		log.Info("kafka broker will be selected randomly across the cluster")
	}
}
