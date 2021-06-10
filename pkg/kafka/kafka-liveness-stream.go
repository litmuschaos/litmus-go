package kafka

import (
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LivenessStream generates kafka liveness pod, which continuously validate the liveness of kafka brokers
// and derive the kafka topic leader(candidate for the deletion)
func LivenessStream(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {
	var ordinality string
	var err error

	// Generate a random string as suffix to topic name
	log.Info("[Liveness]: Set the kafka topic name")
	experimentsDetails.RunID = common.GetRunID()
	KafkaTopicName := "topic-" + experimentsDetails.RunID

	log.Info("[Liveness]: Creating the kafka liveness pod")
	if err := CreateLivenessPod(experimentsDetails, KafkaTopicName, clients); err != nil {
		return "", err
	}

	log.Info("[Liveness]: Confirm that the kafka liveness pod is running")
	if err := status.CheckApplicationStatus(experimentsDetails.KafkaNamespace, "name=kafka-liveness-"+experimentsDetails.RunID, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients); err != nil {
		return "", errors.Errorf("liveness pod status check failed, err: %v", err)
	}

	log.Info("[Liveness]: Obtain the leader broker ordinality for the topic (partition) created by kafka-liveness")
	if experimentsDetails.KafkaInstanceName == "" {

		execCommandDetails := litmusexec.PodDetails{}
		command := append([]string{"/bin/sh", "-c"}, "kafka-topics --topic topic-"+experimentsDetails.RunID+" --describe --zookeeper "+experimentsDetails.ZookeeperService+":"+experimentsDetails.ZookeeperPort+" | grep -o 'Leader: [^[:space:]]*' | awk '{print $2}'")
		litmusexec.SetExecCommandAttributes(&execCommandDetails, "kafka-liveness-"+experimentsDetails.RunID, "kafka-consumer", experimentsDetails.KafkaNamespace)
		ordinality, err = litmusexec.Exec(&execCommandDetails, clients, command)
		if err != nil {
			return "", errors.Errorf("unable to get ordinality details, err: %v", err)
		}
	} else {
		// It will contains all the pod & container details required for exec command
		execCommandDetails := litmusexec.PodDetails{}

		command := append([]string{"/bin/sh", "-c"}, "kafka-topics --topic topic-"+experimentsDetails.RunID+" --describe --zookeeper "+experimentsDetails.ZookeeperService+":"+experimentsDetails.ZookeeperPort+"/"+experimentsDetails.KafkaInstanceName+" | grep -o 'Leader: [^[:space:]]*' | awk '{print $2}'")
		litmusexec.SetExecCommandAttributes(&execCommandDetails, "kafka-liveness-"+experimentsDetails.RunID, "kafka-consumer", experimentsDetails.KafkaNamespace)
		ordinality, err = litmusexec.Exec(&execCommandDetails, clients, command)
		if err != nil {
			return "", errors.Errorf("unable to get ordinality details, err: %v", err)
		}
	}

	log.Info("[Liveness]: Determine the leader broker pod name")
	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.KafkaNamespace).List(metav1.ListOptions{LabelSelector: experimentsDetails.KafkaLabel})
	if err != nil {
		return "", errors.Errorf("unable to find the pods with matching labels, err: %v", err)
	}

	for _, pod := range podList.Items {
		if strings.ContainsAny(pod.Name, ordinality) {
			return pod.Name, nil
		}
	}

	return "", errors.Errorf("no kafka pod found with %v ordinality", ordinality)
}

// CreateLivenessPod creates the kafka liveness pod
func CreateLivenessPod(experimentsDetails *experimentTypes.ExperimentDetails, KafkaTopicName string, clients clients.ClientSets) error {

	LivenessPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "kafka-liveness-" + experimentsDetails.RunID,
			Labels: map[string]string{
				"app":                       "kafka-liveness",
				"name":                      "kafka-liveness-" + experimentsDetails.RunID,
				"app.kubernetes.io/part-of": "litmus",
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:  "kafka-topic-creator",
					Image: experimentsDetails.KafkaLivenessImage,
					Command: []string{
						"sh",
						"-c",
						"./topic.sh",
					},
					Env: []corev1.EnvVar{
						{
							Name:  "TOPIC_NAME",
							Value: KafkaTopicName,
						},
						{
							Name:  "KAFKA_INSTANCE_NAME",
							Value: experimentsDetails.KafkaInstanceName,
						},
						{
							Name:  "ZOOKEEPER_SERVICE",
							Value: experimentsDetails.ZookeeperService,
						},
						{
							Name:  "ZOOKEEPER_PORT",
							Value: experimentsDetails.ZookeeperPort,
						},
						{
							Name:  "REPLICATION_FACTOR",
							Value: experimentsDetails.KafkaRepliationFactor,
						},
					},
					ImagePullPolicy: corev1.PullPolicy("Always"),
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "kafka-producer",
					Image: experimentsDetails.KafkaLivenessImage,
					Command: []string{
						"sh",
						"-c",
						"stdbuf -oL ./producer.sh",
					},
					Env: []corev1.EnvVar{
						{
							Name:  "TOPIC_NAME",
							Value: KafkaTopicName,
						},
						{
							Name:  "KAFKA_SERVICE",
							Value: experimentsDetails.KafkaService,
						},
						{
							Name:  "KAFKA_PORT",
							Value: experimentsDetails.KafkaPort,
						},
					},
					ImagePullPolicy: corev1.PullPolicy("Always"),
				},
				{
					Name:  "kafka-consumer",
					Image: experimentsDetails.KafkaLivenessImage,
					Command: []string{
						"sh",
						"-c",
						"stdbuf -oL ./consumer.sh",
					},
					Env: []corev1.EnvVar{
						{
							Name:  "KAFKA_CONSUMER_TIMEOUT",
							Value: strconv.Itoa(experimentsDetails.KafkaConsumerTimeout),
						},
						{
							Name:  "TOPIC_NAME",
							Value: KafkaTopicName,
						},
						{
							Name:  "KAFKA_SERVICE",
							Value: experimentsDetails.KafkaService,
						},
						{
							Name:  "KAFKA_PORT",
							Value: experimentsDetails.KafkaPort,
						},
					},
					ImagePullPolicy: corev1.PullPolicy("Always"),
				},
			},
			RestartPolicy: corev1.RestartPolicy("Never"),
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.KafkaNamespace).Create(LivenessPod)
	if err != nil {
		return errors.Errorf("unable to create Liveness pod, err: %v", err)
	}
	return nil
}
