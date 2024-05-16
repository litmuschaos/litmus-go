package environment

import (
	"strconv"

	exp "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	kafkaTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// GetENV fetches all the env variables from the runner pod
func GetENV(kafkaDetails *kafkaTypes.ExperimentDetails) {

	var ChaoslibDetail exp.ExperimentDetails

	ChaoslibDetail.ExperimentName = types.Getenv("EXPERIMENT_NAME", "kafka-broker-pod-failure")
	ChaoslibDetail.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	ChaoslibDetail.EngineName = types.Getenv("CHAOSENGINE", "")
	ChaoslibDetail.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "60"))
	ChaoslibDetail.ChaosInterval = types.Getenv("CHAOS_INTERVAL", "10")
	ChaoslibDetail.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	ChaoslibDetail.ChaosServiceAccount = types.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	ChaoslibDetail.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	ChaoslibDetail.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	ChaoslibDetail.InstanceID = types.Getenv("INSTANCE_ID", "")
	ChaoslibDetail.ChaosPodName = types.Getenv("POD_NAME", "")
	ChaoslibDetail.Sequence = types.Getenv("SEQUENCE", "parallel")
	ChaoslibDetail.PodsAffectedPerc = types.Getenv("PODS_AFFECTED_PERC", "0")
	ChaoslibDetail.Force, _ = strconv.ParseBool(types.Getenv("FORCE", "true"))
	ChaoslibDetail.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	ChaoslibDetail.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))

	ChaoslibDetail.AppNS, ChaoslibDetail.AppKind, ChaoslibDetail.AppLabel = getAppDetails()

	kafkaDetails.ChaoslibDetail = &ChaoslibDetail
	kafkaDetails.KafkaKind = types.Getenv("KAFKA_KIND", "statefulset")
	kafkaDetails.KafkaLivenessStream = types.Getenv("KAFKA_LIVENESS_STREAM", "enable")
	kafkaDetails.KafkaLivenessImage = types.Getenv("KAFKA_LIVENESS_IMAGE", "litmuschaos/kafka-client:latest")
	kafkaDetails.KafkaConsumerTimeout, _ = strconv.Atoi(types.Getenv("KAFKA_CONSUMER_TIMEOUT", "60000"))
	kafkaDetails.KafkaInstanceName = types.Getenv("KAFKA_INSTANCE_NAME", "")
	kafkaDetails.KafkaNamespace = types.Getenv("KAFKA_NAMESPACE", "default")
	kafkaDetails.KafkaLabel = types.Getenv("KAFKA_LABEL", "")
	kafkaDetails.KafkaBroker = types.Getenv("KAFKA_BROKER", "")
	kafkaDetails.KafkaRepliationFactor = types.Getenv("KAFKA_REPLICATION_FACTOR", "")
	kafkaDetails.KafkaService = types.Getenv("KAFKA_SERVICE", "")
	kafkaDetails.KafkaPort = types.Getenv("KAFKA_PORT", "9092")
	kafkaDetails.ZookeeperNamespace = types.Getenv("ZOOKEEPER_NAMESPACE", "")
	kafkaDetails.ZookeeperLabel = types.Getenv("ZOOKEEPER_LABEL", "")
	kafkaDetails.ZookeeperService = types.Getenv("ZOOKEEPER_SERVICE", "")
	kafkaDetails.ZookeeperPort = types.Getenv("ZOOKEEPER_PORT", "")
	kafkaDetails.RunID = types.Getenv("RunID", "")

}

func getAppDetails() (string, string, string) {
	targets := types.Getenv("TARGETS", "")
	app := types.GetTargets(targets)
	if len(app) != 0 {
		return app[0].Namespace, app[0].Kind, app[0].Labels[0]
	}
	return "", "", ""
}
