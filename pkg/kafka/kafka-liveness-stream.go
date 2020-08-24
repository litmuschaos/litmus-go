package kafka

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

// LivenessStream will generate kafka liveness deployment on the basic of given condition
func LivenessStream(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {
	var err error
	var ordinality string
	var out bytes.Buffer
	var stderr bytes.Buffer

	// Generate a random string as suffix to topic name
	log.Info("[Liveness]: Set the kafka topic name")
	experimentsDetails.RunID = common.GetRunID()
	KafkaTopicName := "topic-" + experimentsDetails.RunID

	if experimentsDetails.KafkaSaslAuth == "" || experimentsDetails.KafkaSaslAuth == "disabled" {
		log.Info("[Liveness]: Generate the kafka liveness spec from template")
		err = CreateLivenessNonAuth(experimentsDetails, KafkaTopicName, clients)
		if err != nil {
			return "", err
		}
	} else if experimentsDetails.KafkaSaslAuth == "enabled" {

		err = CreateLivenessSaslAuth(experimentsDetails, KafkaTopicName, clients)
		if err != nil {
			return "", err
		}
	}

	log.Info("[Liveness]: Confirm that the kafka liveness pod is running")
	err = status.CheckApplicationStatus(experimentsDetails.KafkaNamespace, "name=kafka-liveness", experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients)
	if err != nil {
		return "", errors.Errorf("Liveness pod status check failed, due to %v", err)
	}

	log.Info("[Liveness]: Obtain the leader broker ordinality for the topic (partition) created by kafka-liveness")
	if experimentsDetails.KafkaInstanceName == "" {

		execCommandDetails := litmusexec.PodDetails{}

		command := append([]string{"/bin/sh", "-c"}, "-- kafka-topics --topic "+KafkaTopicName+" --describe --zookeeper "+experimentsDetails.ZookeeperService+":"+experimentsDetails.ZookeeperPort+" | grep -o 'Leader: [^[:space:]]*' | awk '{print $2}'")
		litmusexec.SetExecCommandAttributes(&execCommandDetails, "kafka-liveness", "kafka-consumer", experimentsDetails.KafkaNamespace)
		ordinality, err = litmusexec.Exec(&execCommandDetails, clients, command)
		if err != nil {
			return "", errors.Errorf("Unable to get ordinality details, due to err: %v", err)
		}
	} else {
		// It will contains all the pod & container details required for exec command
		execCommandDetails := litmusexec.PodDetails{}

		command := append([]string{"/bin/sh", "-c"}, "-- kafka-topics --topic "+KafkaTopicName+" --describe --zookeeper "+experimentsDetails.ZookeeperService+":"+experimentsDetails.ZookeeperPort+"/"+experimentsDetails.KafkaInstanceName+" | grep -o 'Leader: [^[:space:]]*' | awk '{print $2}'")
		litmusexec.SetExecCommandAttributes(&execCommandDetails, "kafka-liveness", "kafka-consumer", experimentsDetails.KafkaNamespace)
		ordinality, err = litmusexec.Exec(&execCommandDetails, clients, command)
		if err != nil {
			return "", errors.Errorf("Unable to get ordinality details, due to err: %v", err)
		}

	}
	log.Info("[Liveness]: Determine the leader broker pod name")
	cmd := exec.Command("sh", "-c", `kubectl get pods -l `+experimentsDetails.KafkaLabel+` --no-headers -o custom-columns=:metadata.name -n `+experimentsDetails.KafkaNamespace+` | grep '^.*-`+ordinality+`$'`)
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		klog.Infof(fmt.Sprint(err) + ": " + stderr.String())
		klog.Infof("Error: %v", err)
		return "", errors.Errorf("Fail to get leader broker pod name, due to %v", err)
	}
	response := fmt.Sprint(out.String())
	LivenessTopicLeader := response

	return LivenessTopicLeader, nil
}

// CreateLivenessNonAuth will create a liveness pod when kafka saslAuth in not enabled
func CreateLivenessNonAuth(experimentsDetails *experimentTypes.ExperimentDetails, KafkaTopicName string, clients clients.ClientSets) error {

	LivenessNonAuth := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "kafka-liveness",
			Labels: map[string]string{
				"name": "kafka-liveness",
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				corev1.Container{
					Name:  "kafka-topic-creator",
					Image: experimentsDetails.KafkaLivenessImage,
					Command: []string{
						"sh",
						"-c",
						"./topic.sh",
					},
					Env: []corev1.EnvVar{
						corev1.EnvVar{
							Name:  "TOPIC_NAME",
							Value: KafkaTopicName,
						},
						corev1.EnvVar{
							Name:  "KAFKA_INSTANCE_NAME",
							Value: "" + experimentsDetails.KafkaInstanceName + "",
						},
						corev1.EnvVar{
							Name:  "ZOOKEEPER_SERVICE",
							Value: experimentsDetails.ZookeeperService,
						},
						corev1.EnvVar{
							Name:  "ZOOKEEPER_PORT",
							Value: experimentsDetails.ZookeeperPort,
						},
						corev1.EnvVar{
							Name:  "REPLICATION_FACTOR",
							Value: experimentsDetails.KafkaRepliationFactor,
						},
					},
					Resources:       corev1.ResourceRequirements{},
					ImagePullPolicy: corev1.PullPolicy("Always"),
				},
			},
			Containers: []corev1.Container{
				corev1.Container{
					Name:  "kafka-producer",
					Image: experimentsDetails.KafkaLivenessImage,
					Command: []string{
						"sh",
						"-c",
						"stdbuf -oL ./producer.sh",
					},
					Env: []corev1.EnvVar{
						corev1.EnvVar{
							Name:  "TOPIC_NAME",
							Value: KafkaTopicName,
						},
						corev1.EnvVar{
							Name:  "KAFKA_SERVICE",
							Value: experimentsDetails.KafkaService,
						},
						corev1.EnvVar{
							Name:  "KAFKA_PORT",
							Value: experimentsDetails.KafkaPort,
						},
					},
					Resources:       corev1.ResourceRequirements{},
					ImagePullPolicy: corev1.PullPolicy("Always"),
				},
				corev1.Container{
					Name:  "kafka-consumer",
					Image: experimentsDetails.KafkaLivenessImage,
					Command: []string{
						"sh",
						"-c",
						"stdbuf -oL ./consumer.sh",
					},
					Env: []corev1.EnvVar{
						corev1.EnvVar{
							Name:  "KAFKA_CONSUMER_TIMEOUT",
							Value: strconv.Itoa(experimentsDetails.KafkaConsumerTimeout),
						},
						corev1.EnvVar{
							Name:  "TOPIC_NAME",
							Value: KafkaTopicName,
						},
						corev1.EnvVar{
							Name:  "KAFKA_SERVICE",
							Value: experimentsDetails.KafkaService,
						},
						corev1.EnvVar{
							Name:  "KAFKA_PORT",
							Value: experimentsDetails.KafkaPort,
						},
					},
					Resources:       corev1.ResourceRequirements{},
					ImagePullPolicy: corev1.PullPolicy("Always"),
				},
			},
			RestartPolicy: corev1.RestartPolicy("Never"),
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.KafkaNamespace).Create(LivenessNonAuth)
	if err != nil {
		return errors.Errorf("Unable to create Liveness Non Auth pod, due to %v", err)
	}
	return nil

}

// CreateLivenessSaslAuth will create a liveness pod Sasl auth in enabled
func CreateLivenessSaslAuth(experimentsDetails *experimentTypes.ExperimentDetails, KafkaTopicName string, clients clients.ClientSets) error {

	LivenessSaslAuth := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "kafka-liveness",
			Labels: map[string]string{
				"name": "kafka-liveness",
			},
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				corev1.Volume{
					Name: "jaas-properties",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "jaas-properties",
							},
						},
					},
				},
				corev1.Volume{
					Name: "client-properties",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "client-properties",
							},
						},
					},
				},
				corev1.Volume{
					Name: "jaas-conf",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "jaas-conf",
							},
						},
					},
				},
			},
			InitContainers: []corev1.Container{
				corev1.Container{
					Name:  "kafka-topic-creator",
					Image: experimentsDetails.KafkaLivenessImage,
					Command: []string{
						"sh",
						"-c",
						"./topic.sh",
					},
					Env: []corev1.EnvVar{
						corev1.EnvVar{
							Name:  "TOPIC_NAME",
							Value: KafkaTopicName,
						},
						corev1.EnvVar{
							Name:  "KAFKA_INSTANCE_NAME",
							Value: experimentsDetails.KafkaInstanceName,
						},
						corev1.EnvVar{
							Name:  "ZOOKEEPER_SERVICE",
							Value: experimentsDetails.ZookeeperService,
						},
						corev1.EnvVar{
							Name:  "ZOOKEEPER_PORT",
							Value: experimentsDetails.ZookeeperPort,
						},
						corev1.EnvVar{
							Name:  "REPLICATION_FACTOR",
							Value: experimentsDetails.KafkaRepliationFactor,
						},
					},
					Resources:       corev1.ResourceRequirements{},
					ImagePullPolicy: corev1.PullPolicy("Always"),
				},
			},
			Containers: []corev1.Container{
				corev1.Container{
					Name:  "kafka-producer",
					Image: experimentsDetails.KafkaLivenessImage,
					Command: []string{
						"sh",
						"-c",
						"stdbuf -oL ./producer.sh",
					},
					Env: []corev1.EnvVar{
						corev1.EnvVar{
							Name:  "TOPIC_NAME",
							Value: KafkaTopicName,
						},
						corev1.EnvVar{
							Name:  "KAFKA_SERVICE",
							Value: experimentsDetails.KafkaService,
						},
						corev1.EnvVar{
							Name:  "KAFKA_PORT",
							Value: experimentsDetails.KafkaPort,
						},
						corev1.EnvVar{
							Name:  "KAFKA_OPTS",
							Value: "-Djava.security.auth.login.config=/opt/jaas.conf",
						},
					},
					Resources: corev1.ResourceRequirements{},
					VolumeMounts: []corev1.VolumeMount{
						corev1.VolumeMount{
							Name:      "jaas-properties",
							MountPath: "/etc",
						},
						corev1.VolumeMount{
							Name:      "client-properties",
							MountPath: "/opt",
						},
						corev1.VolumeMount{
							Name:      "jaas-conf",
							MountPath: "/opt",
						},
					},
					ImagePullPolicy: corev1.PullPolicy("Always"),
				},
				corev1.Container{
					Name:  "kafka-consumer",
					Image: experimentsDetails.KafkaLivenessImage,
					Command: []string{
						"sh",
						"-c",
						"stdbuf -oL ./consumer.sh",
					},
					Env: []corev1.EnvVar{
						corev1.EnvVar{
							Name:  "KAFKA_CONSUMER_TIMEOUT",
							Value: strconv.Itoa(experimentsDetails.KafkaConsumerTimeout),
						},
						corev1.EnvVar{
							Name:  "TOPIC_NAME",
							Value: KafkaTopicName,
						},
						corev1.EnvVar{
							Name:  "KAFKA_SERVICE",
							Value: experimentsDetails.KafkaService,
						},
						corev1.EnvVar{
							Name:  "KAFKA_PORT",
							Value: experimentsDetails.KafkaPort,
						},
						corev1.EnvVar{
							Name:  "KAFKA_OPTS",
							Value: "-Djava.security.auth.login.config=/opt/jaas.conf",
						},
					},
					Resources: corev1.ResourceRequirements{},
					VolumeMounts: []corev1.VolumeMount{
						corev1.VolumeMount{
							Name:      "jaas-properties",
							MountPath: "/etc",
						},
						corev1.VolumeMount{
							Name:      "client-properties",
							MountPath: "/opt",
						},
						corev1.VolumeMount{
							Name:      "jaas-conf",
							MountPath: "/opt",
						},
					},
					ImagePullPolicy: corev1.PullPolicy("Always"),
				},
			},
			RestartPolicy: corev1.RestartPolicy("Never"),
		},
	}
	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.KafkaNamespace).Create(LivenessSaslAuth)
	if err != nil {
		return errors.Errorf("Unable to create Liveness Non Auth pod, due to %v", err)
	}
	return nil
}
