package kafka

import (
	"math/rand"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
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
		log.Errorf("Kafka pod status check failed err: %v", err)
		return err
	}

	log.Info("[Status]: Verify that all zookeeper pods are running")
	err = status.CheckApplicationStatus(experimentsDetails.ZookeeperNamespace, experimentsDetails.ZookeeperLabel, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients)
	if err != nil {
		log.Errorf("Zookeeper status check failed err: %v", err)
		return err
	}

	return nil
}

// GetRandomPodName will return the first pod name from the list of pods obtained from label and namespace
func GetRandomPodName(PodNamespace, PodLabel string, clients clients.ClientSets) (string, error) {

	podList, err := clients.KubeClient.CoreV1().Pods(PodNamespace).List(metav1.ListOptions{LabelSelector: PodLabel})
	if err != nil {
		return "", errors.Errorf("unable to find the pods err: %v", err)
	}
	rand.Seed(time.Now().Unix())
	randomIndex := rand.Intn(len(podList.Items))
	return podList.Items[randomIndex].Name, nil
}

// GetReplicaCount will return the number of replicas present
func GetReplicaCount(PodNamespace, PodLabel string, clients clients.ClientSets) (int, error) {
	PodList, err := clients.KubeClient.CoreV1().Pods(PodNamespace).List(metav1.ListOptions{LabelSelector: PodLabel})
	if err != nil {
		return 0, errors.Errorf("Unable to find the pods err: %v", err)
	}

	return len(PodList.Items), nil
}
