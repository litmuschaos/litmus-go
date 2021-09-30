package cassandra

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/cassandra/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LivenessCheck will create an external liveness pod which will continuously check for the liveness of cassandra statefulset
func LivenessCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {

	// Generate the run_id for the liveness pod
	experimentsDetails.RunID = common.GetRunID()

	// Creating liveness deployment
	if err := CreateLivenessPod(experimentsDetails, clients); err != nil {
		return "", err
	}

	// Creating liveness service
	if err := CreateLivenessService(experimentsDetails, clients); err != nil {
		return "", err
	}

	// Checking the status of liveness deployment pod
	log.Info("[Status]: Checking the status of the cassandra liveness pod")
	if err := status.CheckApplicationStatus(experimentsDetails.ChaoslibDetail.AppNS, "name=cassandra-liveness-deploy-"+experimentsDetails.RunID, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients); err != nil {
		return "", errors.Errorf("liveness pod is not in running state, err: %v", err)
	}

	// Record cassandra liveness pod resource version
	ResourceVersionBefore, err := GetLivenessPodResourceVersion(experimentsDetails, clients)
	if err != nil {
		return ResourceVersionBefore, errors.Errorf("failed to get the pod resource version, err: %v", err)
	}

	return ResourceVersionBefore, nil
}

// LivenessCleanup will check the status of liveness pod cycle and wait till the cycle comes to the complete state
// At last it removes/cleanup the liveness deploy and svc
func LivenessCleanup(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, ResourceVersionBefore string) error {

	// Getting ClusterIP
	log.Info("[CleanUP]: Getting ClusterIP of liveness service")
	ClusterIP, err := GetServiceClusterIP(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("failed to get the ClusterIP of liveness service, err: %v", err)
	}

	// Record cassandra liveness pod resource version after chaos
	ResourceVersionAfter, err := GetLivenessPodResourceVersion(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("failed to get the pod resource version, err: %v", err)
	}

	if err = ResourceVersionCheck(ResourceVersionBefore, ResourceVersionAfter); err != nil {
		return err
	}

	if err = WaitTillCycleComplete(experimentsDetails, ClusterIP); err != nil {
		return errors.Errorf("cycle complete test failed, err: %v", err)
	}

	log.Info("[Cleanup]: Deleting cassandra liveness deployment & service")
	if err = DeleteLivenessDeployment(experimentsDetails, clients); err != nil {
		return errors.Errorf("liveness deployment deletion failed, err: %v", err)
	}
	if err = DeleteLivenessService(experimentsDetails, clients); err != nil {
		return errors.Errorf("liveness service deletion failed, err: %v", err)
	}

	log.Info("[Cleanup]: Cassandra liveness service has been deleted successfully")

	return nil
}

// GetLivenessPodResourceVersion will return the resource version of the liveness pod
func GetLivenessPodResourceVersion(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {

	livenessPods, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaoslibDetail.AppNS).List(metav1.ListOptions{LabelSelector: "name=cassandra-liveness-deploy-" + experimentsDetails.RunID})
	if err != nil {
		return "", errors.Errorf("unable to get the liveness pod, err: %v", err)
	} else if len(livenessPods.Items) == 0 {
		return "", errors.Errorf("No liveness pod found with matching labels")
	}
	ResourceVersion := livenessPods.Items[0].ResourceVersion

	return ResourceVersion, nil
}

// GetServiceClusterIP will return the cluster IP of the liveness service
func GetServiceClusterIP(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {

	service, err := clients.KubeClient.CoreV1().Services(experimentsDetails.ChaoslibDetail.AppNS).Get("cassandra-liveness-service-"+experimentsDetails.RunID, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	return service.Spec.ClusterIP, nil
}

// WaitTillCycleComplete will check the status of liveness pod cycle
// Wait till the cycle come to the complete state
func WaitTillCycleComplete(experimentsDetails *experimentTypes.ExperimentDetails, ClusterIP string) error {

	port := strconv.Itoa(experimentsDetails.LivenessServicePort)
	URL := "http://" + ClusterIP + ":" + port
	log.Infof("The URL to check the status of liveness pod cycle, url: %v", URL)

	return retry.
		Times(uint(experimentsDetails.ChaoslibDetail.Timeout / experimentsDetails.ChaoslibDetail.Delay)).
		Wait(time.Duration(experimentsDetails.ChaoslibDetail.Delay) * time.Second).
		Try(func(attempt uint) error {
			response, err := http.Get(URL)
			if err != nil {
				return errors.Errorf("the HTTP request failed with error %s", err)
			}
			data, _ := ioutil.ReadAll(response.Body)
			if !strings.Contains(string(data), "CycleComplete") {
				log.Info("[Verification]: Wait for liveness pod to come in CycleComplete state")
				return errors.Errorf("livenss pod is not in completed state")
			}
			log.Info("Liveness pod comes to CycleComplete state")
			return nil
		})
}

// ResourceVersionCheck compare the resource version for target pods before and after chaos
func ResourceVersionCheck(ResourceVersionBefore, ResourceVersionAfter string) error {

	if ResourceVersionBefore != ResourceVersionAfter {
		return errors.Errorf("resource version check failed, Resource version remains same")
	}
	log.Info("The cassandra cluster is active")

	return nil
}

// DeleteLivenessDeployment deletes the livenes deployments and wait for its termination
func DeleteLivenessDeployment(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	deletePolicy := metav1.DeletePropagationForeground
	if err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.ChaoslibDetail.AppNS).Delete("cassandra-liveness-deploy-"+experimentsDetails.RunID, &metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		return err
	}
	return retry.
		Times(uint(experimentsDetails.ChaoslibDetail.Timeout / experimentsDetails.ChaoslibDetail.Delay)).
		Wait(time.Duration(experimentsDetails.ChaoslibDetail.Delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.ChaoslibDetail.AppNS).List(metav1.ListOptions{LabelSelector: "name=cassandra-liveness-deploy-" + experimentsDetails.RunID})
			if err != nil {
				return errors.Errorf("liveness deployment is not deleted yet, err: %v", err)
			} else if len(podSpec.Items) != 0 {
				return errors.Errorf("liveness deployment is not deleted yet")
			}
			return nil
		})
}

// DeleteLivenessService deletes the liveness service and wait for its termination
func DeleteLivenessService(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	deletePolicy := metav1.DeletePropagationForeground
	if err := clients.KubeClient.CoreV1().Services(experimentsDetails.ChaoslibDetail.AppNS).Delete("cassandra-liveness-service-"+experimentsDetails.RunID, &metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		return errors.Errorf("fail to delete liveness service, err: %v", err)
	}
	return retry.
		Times(uint(experimentsDetails.ChaoslibDetail.Timeout / experimentsDetails.ChaoslibDetail.Delay)).
		Wait(time.Duration(experimentsDetails.ChaoslibDetail.Delay) * time.Second).
		Try(func(attempt uint) error {
			svc, err := clients.KubeClient.CoreV1().Services(experimentsDetails.ChaoslibDetail.AppNS).List(metav1.ListOptions{LabelSelector: "name=cassandra-liveness-service-" + experimentsDetails.RunID})
			if err != nil {
				return errors.Errorf("liveness service is not deleted yet, err: %v", err)
			} else if len(svc.Items) != 0 {
				return errors.Errorf("liveness service is not deleted yet")
			}
			return nil
		})
}

// CreateLivenessPod will create a cassandra liveness deployment
func CreateLivenessPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	// Create liveness deploy
	liveness := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cassandra-liveness-deploy-" + experimentsDetails.RunID,
			Labels: map[string]string{
				"name": "cassandra-liveness-deploy-" + experimentsDetails.RunID,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: func(i int32) *int32 { return &i }(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "cassandra-liveness-deploy-" + experimentsDetails.RunID,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": "cassandra-liveness-deploy-" + experimentsDetails.RunID,
					},
				},
				Spec: apiv1.PodSpec{
					Volumes: []apiv1.Volume{
						{
							Name: "status-volume",
							VolumeSource: apiv1.VolumeSource{
								EmptyDir: &apiv1.EmptyDirVolumeSource{},
							},
						},
					},
					Containers: []apiv1.Container{
						{
							Name:  "liveness-business-logic",
							Image: experimentsDetails.CassandraLivenessImage,
							Command: []string{
								"/bin/bash",
							},
							Args: []string{
								"-c",
								"bash cassandra-liveness-check.sh",
							},
							Env: []apiv1.EnvVar{
								{
									Name:  "LIVENESS_PERIOD_SECONDS",
									Value: "10",
								},
								{
									Name:  "LIVENESS_TIMEOUT_SECONDS",
									Value: "10",
								},
								{
									Name:  "LIVENESS_RETRY_COUNT",
									Value: "10",
								},
								{
									Name:  "CASSANDRA_SVC_NAME",
									Value: experimentsDetails.CassandraServiceName,
								},
								{
									Name:  "REPLICATION_FACTOR",
									Value: experimentsDetails.KeySpaceReplicaFactor,
								},
								{
									Name:  "CASSANDRA_PORT",
									Value: strconv.Itoa(experimentsDetails.CassandraPort),
								},
							},
							Resources: apiv1.ResourceRequirements{},
							VolumeMounts: []apiv1.VolumeMount{
								{
									Name:      "status-volume",
									MountPath: "/var/tmp",
								},
							},
							ImagePullPolicy: apiv1.PullPolicy("Always"),
						},
						{
							Name:  "webserver",
							Image: experimentsDetails.CassandraLivenessImage,
							Command: []string{
								"/bin/bash",
							},
							Args: []string{
								"-c",
								"bash webserver.sh",
							},
							Ports: []apiv1.ContainerPort{
								{
									HostPort:      0,
									ContainerPort: int32(experimentsDetails.LivenessServicePort),
								},
							},
							Env: []apiv1.EnvVar{
								{
									Name:  "INIT_WAIT_SECONDS",
									Value: "10",
								},
								{
									Name:  "LIVENESS_SVC_PORT",
									Value: strconv.Itoa(experimentsDetails.LivenessServicePort),
								},
							},
							Resources: apiv1.ResourceRequirements{},
							VolumeMounts: []apiv1.VolumeMount{
								{
									Name:      "status-volume",
									MountPath: "/var/tmp",
								},
							},
							ImagePullPolicy: apiv1.PullPolicy("Always"),
						},
					},
				},
			},
			Strategy:        appsv1.DeploymentStrategy{},
			MinReadySeconds: 0,
		},
	}

	// Creating liveness deployment
	_, err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.ChaoslibDetail.AppNS).Create(liveness)
	if err != nil {
		return err
	}
	log.Info("Liveness Deployment Created successfully!")
	return nil
}

// CreateLivenessService will create Cassandra liveness service
func CreateLivenessService(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	// Create resource object
	livenessSvc := &apiv1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cassandra-liveness-service-" + experimentsDetails.RunID,
			Labels: map[string]string{
				"name": "cassandra-liveness-service-" + experimentsDetails.RunID,
			},
		},
		Spec: apiv1.ServiceSpec{
			Ports: []apiv1.ServicePort{
				{
					Name:     "liveness",
					Protocol: apiv1.Protocol("TCP"),
					Port:     int32(experimentsDetails.LivenessServicePort),
				},
			},
			Selector: map[string]string{
				"name": "cassandra-liveness-deploy-" + experimentsDetails.RunID,
			},
			HealthCheckNodePort: 0,
		},
	}

	// Creating liveness service
	_, err := clients.KubeClient.CoreV1().Services(experimentsDetails.ChaoslibDetail.AppNS).Create(livenessSvc)
	if err != nil {
		return err
	}
	log.Info("Liveness service created successfully!")

	return nil
}
