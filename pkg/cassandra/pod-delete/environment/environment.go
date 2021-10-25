package environment

import (
	"strconv"

	cassandraTypes "github.com/litmuschaos/litmus-go/pkg/cassandra/pod-delete/types"
	exp "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(cassandraDetails *cassandraTypes.ExperimentDetails) {

	var ChaoslibDetail exp.ExperimentDetails

	ChaoslibDetail.ExperimentName = types.Getenv("EXPERIMENT_NAME", "cassandra-pod-delete")
	ChaoslibDetail.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	ChaoslibDetail.EngineName = types.Getenv("CHAOSENGINE", "")
	ChaoslibDetail.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	ChaoslibDetail.ChaosInterval = types.Getenv("CHAOS_INTERVAL", "10")
	ChaoslibDetail.RampTime, _ = strconv.Atoi(types.Getenv("RAMP_TIME", "0"))
	ChaoslibDetail.ChaosLib = types.Getenv("LIB", "litmus")
	ChaoslibDetail.ChaosServiceAccount = types.Getenv("CHAOS_SERVICE_ACCOUNT", "")
	ChaoslibDetail.AppNS = types.Getenv("APP_NAMESPACE", "")
	ChaoslibDetail.AppLabel = types.Getenv("APP_LABEL", "")
	ChaoslibDetail.AppKind = types.Getenv("APP_KIND", "")
	ChaoslibDetail.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	ChaoslibDetail.InstanceID = types.Getenv("INSTANCE_ID", "")
	ChaoslibDetail.ChaosPodName = types.Getenv("POD_NAME", "")
	ChaoslibDetail.TargetContainer = types.Getenv("TARGET_CONTAINER", "")
	ChaoslibDetail.Force, _ = strconv.ParseBool(types.Getenv("FORCE", "false"))
	ChaoslibDetail.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	ChaoslibDetail.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	ChaoslibDetail.PodsAffectedPerc, _ = strconv.Atoi(types.Getenv("PODS_AFFECTED_PERC", "0"))
	ChaoslibDetail.Sequence = types.Getenv("SEQUENCE", "parallel")
	cassandraDetails.ChaoslibDetail = &ChaoslibDetail
	cassandraDetails.CassandraServiceName = types.Getenv("CASSANDRA_SVC_NAME", "")
	cassandraDetails.KeySpaceReplicaFactor = types.Getenv("KEYSPACE_REPLICATION_FACTOR", "")
	cassandraDetails.CassandraPort, _ = strconv.Atoi(types.Getenv("CASSANDRA_PORT", "9042"))
	cassandraDetails.LivenessServicePort, _ = strconv.Atoi(types.Getenv("LIVENESS_SVC_PORT", "8088"))
	cassandraDetails.CassandraLivenessImage = types.Getenv("CASSANDRA_LIVENESS_IMAGE", "litmuschaos/cassandra-client:latest")
	cassandraDetails.CassandraLivenessCheck = types.Getenv("CASSANDRA_LIVENESS_CHECK", "")
	cassandraDetails.RunID = types.Getenv("RunID", "")
}
