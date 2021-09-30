package environment

import (
	"strconv"

	cassandraTypes "github.com/litmuschaos/litmus-go/pkg/cassandra/pod-delete/types"
	exp "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(cassandraDetails *cassandraTypes.ExperimentDetails) {

	var ChaoslibDetail exp.ExperimentDetails

	ChaoslibDetail.ExperimentName = common.Getenv("EXPERIMENT_NAME", "cassandra-pod-delete")
	ChaoslibDetail.ChaosNamespace = common.Getenv("CHAOS_NAMESPACE", "litmus")
	ChaoslibDetail.EngineName = common.Getenv("CHAOSENGINE", "")
	ChaoslibDetail.ChaosDuration, _ = strconv.Atoi(common.Getenv("TOTAL_CHAOS_DURATION", "30"))
	ChaoslibDetail.ChaosInterval = common.Getenv("CHAOS_INTERVAL", "10")
	ChaoslibDetail.RampTime, _ = strconv.Atoi(common.Getenv("RAMP_TIME", "0"))
	ChaoslibDetail.ChaosLib = common.Getenv("LIB", "litmus")
	ChaoslibDetail.ChaosServiceAccount = common.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	ChaoslibDetail.AppNS = common.Getenv("APP_NAMESPACE", "")
	ChaoslibDetail.AppLabel = common.Getenv("APP_LABEL", "")
	ChaoslibDetail.AppKind = common.Getenv("APP_KIND", "")
	ChaoslibDetail.ChaosUID = clientTypes.UID(common.Getenv("CHAOS_UID", ""))
	ChaoslibDetail.InstanceID = common.Getenv("INSTANCE_ID", "")
	ChaoslibDetail.ChaosPodName = common.Getenv("POD_NAME", "")
	ChaoslibDetail.TargetContainer = common.Getenv("TARGET_CONTAINER", "")
	ChaoslibDetail.Force, _ = strconv.ParseBool(common.Getenv("FORCE", "false"))
	ChaoslibDetail.Delay, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_DELAY", "2"))
	ChaoslibDetail.Timeout, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	ChaoslibDetail.PodsAffectedPerc, _ = strconv.Atoi(common.Getenv("PODS_AFFECTED_PERC", "0"))
	ChaoslibDetail.Sequence = common.Getenv("SEQUENCE", "parallel")
	cassandraDetails.ChaoslibDetail = &ChaoslibDetail
	cassandraDetails.CassandraServiceName = common.Getenv("CASSANDRA_SVC_NAME", "")
	cassandraDetails.KeySpaceReplicaFactor = common.Getenv("KEYSPACE_REPLICATION_FACTOR", "")
	cassandraDetails.CassandraPort, _ = strconv.Atoi(common.Getenv("CASSANDRA_PORT", "9042"))
	cassandraDetails.LivenessServicePort, _ = strconv.Atoi(common.Getenv("LIVENESS_SVC_PORT", "8088"))
	cassandraDetails.CassandraLivenessImage = common.Getenv("CASSANDRA_LIVENESS_IMAGE", "litmuschaos/cassandra-client:latest")
	cassandraDetails.CassandraLivenessCheck = common.Getenv("CASSANDRA_LIVENESS_CHECK", "")
	cassandraDetails.RunID = common.Getenv("RunID", "")
}

//InitialiseChaosVariables initialise all the global variables
func InitialiseChaosVariables(chaosDetails *types.ChaosDetails, cassandraDetails *cassandraTypes.ExperimentDetails) {
	appDetails := types.AppDetails{}
	appDetails.AnnotationCheck, _ = strconv.ParseBool(common.Getenv("ANNOTATION_CHECK", "false"))
	appDetails.AnnotationKey = common.Getenv("ANNOTATION_KEY", "litmuschaos.io/chaos")
	appDetails.AnnotationValue = "true"
	appDetails.Kind = cassandraDetails.ChaoslibDetail.AppKind
	appDetails.Label = cassandraDetails.ChaoslibDetail.AppLabel
	appDetails.Namespace = cassandraDetails.ChaoslibDetail.AppNS

	chaosDetails.ChaosNamespace = cassandraDetails.ChaoslibDetail.ChaosNamespace
	chaosDetails.ChaosPodName = cassandraDetails.ChaoslibDetail.ChaosPodName
	chaosDetails.ChaosUID = cassandraDetails.ChaoslibDetail.ChaosUID
	chaosDetails.EngineName = cassandraDetails.ChaoslibDetail.EngineName
	chaosDetails.ExperimentName = cassandraDetails.ChaoslibDetail.ExperimentName
	chaosDetails.InstanceID = cassandraDetails.ChaoslibDetail.InstanceID
	chaosDetails.Timeout = cassandraDetails.ChaoslibDetail.Timeout
	chaosDetails.Delay = cassandraDetails.ChaoslibDetail.Delay
	chaosDetails.AppDetail = appDetails
	chaosDetails.ProbeImagePullPolicy = cassandraDetails.ChaoslibDetail.LIBImagePullPolicy
	chaosDetails.Randomness, _ = strconv.ParseBool(common.Getenv("RANDOMNESS", "false"))
	chaosDetails.DefaultAppHealthCheck, _ = strconv.ParseBool(common.Getenv("DEFAULT_APP_HEALTH_CHECK", "true"))
}
