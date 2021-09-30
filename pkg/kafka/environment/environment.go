package environment

import (
	"strconv"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	exp "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	kafkaTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(kafkaDetails *kafkaTypes.ExperimentDetails) {

	var ChaoslibDetail exp.ExperimentDetails

	ChaoslibDetail.ExperimentName = common.Getenv("EXPERIMENT_NAME", "kafka-broker-pod-failure")
	ChaoslibDetail.ChaosNamespace = common.Getenv("CHAOS_NAMESPACE", "litmus")
	ChaoslibDetail.EngineName = common.Getenv("CHAOSENGINE", "")
	ChaoslibDetail.ChaosDuration, _ = strconv.Atoi(common.Getenv("TOTAL_CHAOS_DURATION", "60"))
	ChaoslibDetail.ChaosInterval = common.Getenv("CHAOS_INTERVAL", "10")
	ChaoslibDetail.RampTime, _ = strconv.Atoi(common.Getenv("RAMP_TIME", "0"))
	ChaoslibDetail.ChaosLib = common.Getenv("LIB", "litmus")
	ChaoslibDetail.ChaosServiceAccount = common.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	ChaoslibDetail.AppNS = common.Getenv("APP_NAMESPACE", "")
	ChaoslibDetail.AppLabel = common.Getenv("APP_LABEL", "")
	ChaoslibDetail.TargetContainer = common.Getenv("TARGET_CONTAINER", "")
	ChaoslibDetail.AppKind = common.Getenv("APP_KIND", "")
	ChaoslibDetail.ChaosUID = clientTypes.UID(common.Getenv("CHAOS_UID", ""))
	ChaoslibDetail.InstanceID = common.Getenv("INSTANCE_ID", "")
	ChaoslibDetail.ChaosPodName = common.Getenv("POD_NAME", "")
	ChaoslibDetail.Sequence = common.Getenv("SEQUENCE", "parallel")
	ChaoslibDetail.PodsAffectedPerc, _ = strconv.Atoi(common.Getenv("PODS_AFFECTED_PERC", "0"))
	ChaoslibDetail.Force, _ = strconv.ParseBool(common.Getenv("FORCE", "true"))
	ChaoslibDetail.Delay, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_DELAY", "2"))
	ChaoslibDetail.Timeout, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_TIMEOUT", "180"))

	kafkaDetails.ChaoslibDetail = &ChaoslibDetail
	kafkaDetails.KafkaKind = common.Getenv("KAFKA_KIND", "statefulset")
	kafkaDetails.KafkaLivenessStream = common.Getenv("KAFKA_LIVENESS_STREAM", "enable")
	kafkaDetails.KafkaLivenessImage = common.Getenv("KAFKA_LIVENESS_IMAGE", "litmuschaos/kafka-client:latest")
	kafkaDetails.KafkaConsumerTimeout, _ = strconv.Atoi(common.Getenv("KAFKA_CONSUMER_TIMEOUT", "60000"))
	kafkaDetails.KafkaInstanceName = common.Getenv("KAFKA_INSTANCE_NAME", "kafka")
	kafkaDetails.KafkaNamespace = common.Getenv("KAFKA_NAMESPACE", "default")
	kafkaDetails.KafkaLabel = common.Getenv("KAFKA_LABEL", "")
	kafkaDetails.KafkaBroker = common.Getenv("KAFKA_BROKER", "")
	kafkaDetails.KafkaRepliationFactor = common.Getenv("KAFKA_REPLICATION_FACTOR", "")
	kafkaDetails.KafkaService = common.Getenv("KAFKA_SERVICE", "")
	kafkaDetails.KafkaPort = common.Getenv("KAFKA_PORT", "9092")
	kafkaDetails.ZookeeperNamespace = common.Getenv("ZOOKEEPER_NAMESPACE", "")
	kafkaDetails.ZookeeperLabel = common.Getenv("ZOOKEEPER_LABEL", "")
	kafkaDetails.ZookeeperService = common.Getenv("ZOOKEEPER_SERVICE", "")
	kafkaDetails.ZookeeperPort = common.Getenv("ZOOKEEPER_PORT", "")
	kafkaDetails.Lib = common.Getenv("LIB", "litmus")
	kafkaDetails.RunID = common.Getenv("RunID", "")
}

//InitialiseChaosVariables initialise all the global variables
func InitialiseChaosVariables(chaosDetails *types.ChaosDetails, kafkaDetails *kafkaTypes.ExperimentDetails) {
	appDetails := types.AppDetails{}
	appDetails.AnnotationCheck, _ = strconv.ParseBool(common.Getenv("ANNOTATION_CHECK", "false"))
	appDetails.AnnotationKey = common.Getenv("ANNOTATION_KEY", "litmuschaos.io/chaos")
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
	chaosDetails.Randomness, _ = strconv.ParseBool(common.Getenv("RANDOMNESS", "false"))
	chaosDetails.Targets = []v1alpha1.TargetDetails{}
	chaosDetails.DefaultAppHealthCheck, _ = strconv.ParseBool(common.Getenv("DEFAULT_APP_HEALTH_CHECK", "true"))
}
