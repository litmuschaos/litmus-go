package types

import (
	exp "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ChaoslibDetail        *exp.ExperimentDetails
	ExperimentName        string
	KafkaKind             string
	KafkaLivenessStream   string
	KafkaLivenessImage    string
	KafkaConsumerTimeout  int
	KafkaInstanceName     string
	KafkaNamespace        string
	KafkaLabel            string
	KafkaBroker           string
	KafkaRepliationFactor string
	KafkaService          string
	KafkaPort             string
	ZookeeperNamespace    string
	ZookeeperLabel        string
	ZookeeperService      string
	ZookeeperPort         string
	Lib                   string
	RunID                 string
}
