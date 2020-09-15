package kafka

import (
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterHealthCheck will do a health check over a kafka cluster
func ClusterHealthCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	var err error

	// Checking Kafka pods status
	log.Info("[Status]: Verify that all kafka pods are running")
	err = status.CheckApplicationStatus(experimentsDetails.KafkaNamespace, experimentsDetails.KafkaLabel, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients)
	if err != nil {
		log.Errorf("Kafka pod status check failed due to %v\n", err)
		return err
	}

	log.Info("[Status]: Verify that all zookeeper pods are running")
	err = status.CheckApplicationStatus(experimentsDetails.ZookeeperNamespace, experimentsDetails.ZookeeperLabel, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients)
	if err != nil {
		log.Errorf("Zookeeper status check failed due to %v\n", err)
		return err
	}

	log.Info("[Status]: Obtain pod name of any one of the zookeeper pods")
	ZookeeperPodName, err := GetRandomPodName(experimentsDetails.ZookeeperNamespace, experimentsDetails.ZookeeperLabel, clients)
	if err != nil {
		return err
	}

	log.Info("[Status]: Obtain the desired replica count of the Kafka statefulset")
	ReplicaCount, err := GetReplicaCount(experimentsDetails.KafkaNamespace, experimentsDetails.KafkaLabel, clients)
	if err != nil {
		return err
	}

	if experimentsDetails.KafkaInstanceName != "" {

		// It will contains all the pod & container details required for exec command
		execCommandDetails := litmusexec.PodDetails{}

		command := append([]string{"/bin/sh", "-c"}, "zkCli.sh -server "+experimentsDetails.ZookeeperService+":"+experimentsDetails.ZookeeperPort+"/"+experimentsDetails.KafkaInstanceName+" ls /brokers/ids | tail -n 1 | tr -d '[],' | tr ' ' '\n'  | wc -l")
		litmusexec.SetExecCommandAttributes(&execCommandDetails, ZookeeperPodName, "", experimentsDetails.KafkaNamespace)
		kafkaAvailableBrokers, err := litmusexec.Exec(&execCommandDetails, clients, command)
		if err != nil {
			return errors.Errorf("Unable to get kafka available brokers details, due to err: %v", err)
		}
		if strings.Contains(kafkaAvailableBrokers, strconv.Itoa(ReplicaCount)) {
			return errors.Errorf("All Kafka brokers are not alive")
		}
		log.Info("All Kafka brokers are alive")
	}

	return nil
}

// GetRandomPodName will return the first pod name from the list of pods obtained from label and namespace
func GetRandomPodName(PodNamespace, PodLabel string, clients clients.ClientSets) (string, error) {

	PodList, err := clients.KubeClient.CoreV1().Pods(PodNamespace).List(metav1.ListOptions{LabelSelector: PodLabel})
	if err != nil {
		return "", errors.Errorf("unable to get the pods, due to %v", err)
	}
	return PodList.Items[0].Name, nil
}

// GetReplicaCount will return the number of replicas present
func GetReplicaCount(PodNamespace, PodLabel string, clients clients.ClientSets) (int, error) {
	PodList, err := clients.KubeClient.CoreV1().Pods(PodNamespace).List(metav1.ListOptions{LabelSelector: PodLabel})
	if err != nil {
		return 0, errors.Errorf("Unable to get the pods, due to %v", err)
	}

	return len(PodList.Items), nil
}
