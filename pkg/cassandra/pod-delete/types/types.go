package types

import (
	exp "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ChaoslibDetail         *exp.ExperimentDetails
	CassandraServiceName   string
	KeySpaceReplicaFactor  string
	CassandraPort          int
	LivenessServicePort    int
	CassandraLivenessImage string
	CassandraLivenessCheck string
	RunID                  string
	Sequence               string
}
