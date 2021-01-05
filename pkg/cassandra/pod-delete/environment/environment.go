package environment

import (
	"os"
	"strconv"

	cassandraTypes "github.com/litmuschaos/litmus-go/pkg/cassandra/pod-delete/types"
	exp "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(cassandraDetails *cassandraTypes.ExperimentDetails) {

	var ChaoslibDetail exp.ExperimentDetails

	ChaoslibDetail.ExperimentName = Getenv("EXPERIMENT_NAME", "cassandra-pod-delete")
	ChaoslibDetail.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "litmus")
	ChaoslibDetail.EngineName = Getenv("CHAOSENGINE", "")
	ChaoslibDetail.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "30"))
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
	ChaoslibDetail.Force, _ = strconv.ParseBool(Getenv("FORCE", "false"))
	ChaoslibDetail.Delay, _ = strconv.Atoi(Getenv("STATUS_CHECK_DELAY", "2"))
	ChaoslibDetail.Timeout, _ = strconv.Atoi(Getenv("STATUS_CHECK_TIMEOUT", "180"))
	ChaoslibDetail.PodsAffectedPerc, _ = strconv.Atoi(Getenv("PODS_AFFECTED_PERC", "0"))
	cassandraDetails.ChaoslibDetail = &ChaoslibDetail
	cassandraDetails.CassandraServiceName = Getenv("CASSANDRA_SVC_NAME", "")
	cassandraDetails.KeySpaceReplicaFactor = Getenv("KEYSPACE_REPLICATION_FACTOR", "")
	cassandraDetails.CassandraPort, _ = strconv.Atoi(Getenv("CASSANDRA_PORT", "9042"))
	cassandraDetails.LivenessServicePort, _ = strconv.Atoi(Getenv("LIVENESS_SVC_PORT", "8088"))
	cassandraDetails.CassandraLivenessImage = Getenv("CASSANDRA_LIVENESS_IMAGE", "litmuschaos/cassandra-client:latest")
	cassandraDetails.CassandraLivenessCheck = Getenv("CASSANDRA_LIVENESS_CHECK", "")
	cassandraDetails.RunID = Getenv("RunID", "")
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
func InitialiseChaosVariables(chaosDetails *types.ChaosDetails, cassandraDetails *cassandraTypes.ExperimentDetails) {
	appDetails := types.AppDetails{}
	appDetails.AnnotationCheck, _ = strconv.ParseBool(Getenv("ANNOTATION_CHECK", "false"))
	appDetails.AnnotationKey = Getenv("ANNOTATION_KEY", "litmuschaos.io/chaos")
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
	chaosDetails.ProbeImagePullPolicy = experimentDetails.LIBImagePullPolicy
}
