package environment

import (
	"os"
	"strconv"

	exp "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	kafkaTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(kafkaDetails *kafkaTypes.ExperimentDetails) {

	var ChaoslibDetail exp.ExperimentDetails

	ChaoslibDetail.ExperimentName = Getenv("EXPERIMENT_NAME", "kafka-broker-pod-failure")
	ChaoslibDetail.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "litmus")
	ChaoslibDetail.EngineName = Getenv("CHAOSENGINE", "")
	ChaoslibDetail.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "60"))
	ChaoslibDetail.ChaosInterval, _ = strconv.Atoi(Getenv("CHAOS_INTERVAL", "10"))
	ChaoslibDetail.RampTime, _ = strconv.Atoi(Getenv("RAMP_TIME", "0"))
	ChaoslibDetail.ChaosLib = Getenv("LIB", "litmus")
	ChaoslibDetail.ChaosServiceAccount = Getenv("CHAOS_SERVICE_ACCOUNT", "")
	ChaoslibDetail.AppNS = Getenv("APP_NAMESPACE", "")
	ChaoslibDetail.AppLabel = Getenv("APP_LABEL", "")
	ChaoslibDetail.AppKind = Getenv("APP_KIND", "")
	ChaoslibDetail.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	ChaoslibDetail.InstanceID = Getenv("INSTANCE_ID", "")
	ChaoslibDetail.ChaosPodName = Getenv("POD_NAME", "")
	ChaoslibDetail.Force, _ = strconv.ParseBool(Getenv("FORCE", "true"))
	ChaoslibDetail.Delay, _ = strconv.Atoi(Getenv("STATUS_CHECK_DELAY", "2"))
	ChaoslibDetail.Timeout, _ = strconv.Atoi(Getenv("STATUS_CHECK_TIMEOUT", "180"))

	kafkaDetails.ChaoslibDetail = &ChaoslibDetail
	kafkaDetails.KafkaKind = Getenv("KAFKA_KIND", "statefulset")
	kafkaDetails.KafkaLivenessStream = Getenv("KAFKA_LIVENESS_STREAM", "enabled")
	kafkaDetails.KafkaLivenessImage = Getenv("KAFKA_LIVENESS_IMAGE", "litmuschaos/kafka-client:ci")
	kafkaDetails.KafkaConsumerTimeout, _ = strconv.Atoi(Getenv("KAFKA_CONSUMER_TIMEOUT", "60000"))
	kafkaDetails.KafkaInstanceName = Getenv("KAFKA_INSTANCE_NAME", "kafka")
	kafkaDetails.KafkaNamespace = Getenv("KAFKA_NAMESPACE", "default")
	kafkaDetails.KafkaLabel = Getenv("KAFKA_LABEL", "")
	kafkaDetails.KafkaBroker = Getenv("KAFKA_BROKER", "")
	kafkaDetails.KafkaRepliationFactor = Getenv("KAFKA_REPLICATION_FACTOR", "")
	kafkaDetails.KafkaService = Getenv("KAFKA_SERVICE", "")
	kafkaDetails.KafkaPort = Getenv("KAFKA_PORT", "9092")
	kafkaDetails.ZookeeperNamespace = Getenv("ZOOKEEPER_NAMESPACE", "")
	kafkaDetails.ZookeeperLabel = Getenv("ZOOKEEPER_LABEL", "")
	kafkaDetails.ZookeeperService = Getenv("ZOOKEEPER_SERVICE", "")
	kafkaDetails.ZookeeperPort = Getenv("ZOOKEEPER_PORT", "")
	kafkaDetails.Lib = Getenv("LIB", "litmus")
	kafkaDetails.RunID = Getenv("RunID", "")

}

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}

//InitialiseChaosVariables initialise all the global variables
func InitialiseChaosVariables(chaosDetails *types.ChaosDetails, kafkaDetails *kafkaTypes.ExperimentDetails) {
	appDetails := types.AppDetails{}
	appDetails.AnnotationCheck, _ = strconv.ParseBool(Getenv("ANNOTATION_CHECK", "false"))
	appDetails.AnnotationKey = Getenv("ANNOTATION_KEY", "litmuschaos.io/chaos")
	appDetails.AnnotationValue = "true"
	appDetails.Kind = kafkaDetails.ChaoslibDetail.AppKind
	appDetails.Label = kafkaDetails.ChaoslibDetail.AppLabel
	appDetails.Namespace = kafkaDetails.ChaoslibDetail.AppNS

	chaosDetails.ChaosNamespace = kafkaDetails.ChaoslibDetail.ChaosNamespace
	chaosDetails.ChaosPodName = kafkaDetails.ChaoslibDetail.ChaosPodName
	chaosDetails.ChaosUID = kafkaDetails.ChaoslibDetail.ChaosUID
	chaosDetails.EngineName = kafkaDetails.ChaoslibDetail.EngineName
	chaosDetails.ExperimentName = kafkaDetails.ChaoslibDetail.ExperimentName
	chaosDetails.InstanceID = kafkaDetails.ChaoslibDetail.InstanceID
	chaosDetails.Timeout = kafkaDetails.ChaoslibDetail.Timeout
	chaosDetails.Delay = kafkaDetails.ChaoslibDetail.Delay
	chaosDetails.AppDetail = appDetails
	chaosDetails.ProbeImagePullPolicy = kafkaDetails.ChaoslibDetail.LIBImagePullPolicy
}
