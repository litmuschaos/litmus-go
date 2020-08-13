package cassandra

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/litmuschaos/litmus-go/chaoslib/litmus/pod_delete"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/openebs/maya/pkg/util/retry"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog"
)

// NodeToolStatusCheck checks for the distrubution of the load on the ring
func NodeToolStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	var err error
	var replicaCount int

	// Getting application pod list
	targetPodList, err := pod_delete.PreparePodList(experimentsDetails, clients)
	if err != nil {
		return err
	}
	log.Infof("[NodeToolStatus]: The application pod name for checking load distribution: %v", targetPodList[0])

	replicaCount, err = GetApplicationReplicaCount(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the application name and application nodename due to, err: %v", err)
	}
	log.Info("[Check]: Checking for the distribution of load on the ring")

	// Get the load percentage on the application pod
	loadPercentage, err := GetLoadDistrubution(experimentsDetails, targetPodList[0])
	if err != nil {
		return errors.Errorf("Load distribution check failed, due to %v", err)
	}

	// Check the load precentage
	if err = CheckLoadPercentage(loadPercentage, replicaCount); err != nil {
		return errors.Errorf("[NodeToolStatus]: Load percentage check failed, due to %v", err)
	}

	return nil
}

// LivenessCheck will check the liveness of the cassandra application during chaos
func LivenessCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) ([]string, error) {
	var err error
	var emptyArray []string

	// Creating liveness deployment
	err = CreateLivenessPod(experimentsDetails, clients)
	if err != nil {
		log.Errorf("Liveness check failed, due to %v", err)
	}

	// Creating liveness service
	err = CreateLivenessService(experimentsDetails, clients)
	if err != nil {
		log.Errorf("[Liveness]: Liveness Service Check failed, due to %v", err)
	}

	// Checking the status of liveness deployment pod
	log.Info("[Status]: Checking the status of the cassandra liveness pod")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, "name=cassandra-liveness-deploy", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return emptyArray, errors.Errorf("[Liveness]: Liveness pod is not in running state, err: %v", err)
	}

	// Record cassandra liveness pod resource version
	log.Info("[Liveness]: Record the resource version of liveness pod")
	targetPodList, err := pod_delete.PreparePodList(experimentsDetails, clients)
	if err != nil {
		return emptyArray, err
	}
	klog.Infof("The targetPodList is: %v", targetPodList)
	ResourceVersionBefore := make([]string, len(targetPodList))

	ResourceVersionBefore, err = GetPodResourceVersion(experimentsDetails, clients, targetPodList)
	if err != nil {
		return ResourceVersionBefore, errors.Errorf("Fail to get the pod resource version, due to %v", err)
	}
	log.Info("Resource Version before chaos recorded successfully")

	return ResourceVersionBefore, nil
}

// LivenessCleanup will check the status of liveness pod cycle and wait till the cycle comes to the complete state
// At last it removes/cleanup the liveness deploy and svc
func LivenessCleanup(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, ResourceVersionBefore []string) error {

	var err error
	targetPodList, err := pod_delete.PreparePodList(experimentsDetails, clients)
	if err != nil {
		return err
	}

	// Record cassandra liveness pod resource version after chaos
	log.Info("[Liveness]: Record the resource version of liveness pod (after-chaos)")

	ResourceVersionAfter := make([]string, len(targetPodList))
	ResourceVersionAfter, err = GetPodResourceVersion(experimentsDetails, clients, targetPodList)
	if err != nil {
		return errors.Errorf("Fail to get the pod resource version")
	}

	err = ResourceVersionCheck(ResourceVersionBefore, ResourceVersionAfter)
	if err != nil {
		return err
	}

	err = DeleteLivenessDeployment(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Liveness deployment deletion failed, due to %v", err)
	}

	log.Info("Cassandra liveness deployment deleted successfully")

	err = DeleteLivenessService(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Liveness service deletion failed, due to %v", err)
	}

	log.Info("Cassandra liveness service deleted successfully")

	return nil
}

//GetApplicationReplicaCount will return the replica count of the sts application
func GetApplicationReplicaCount(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (int, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(metav1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(podList.Items) == 0 {
		return 0, errors.Errorf("Fail to get the application pod in %v namespace", experimentsDetails.AppNS)
	}

	return len(podList.Items), nil
}

// GetPodResourceVersion will return the resource version of the application pods
func GetPodResourceVersion(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, targetPodList []string) ([]string, error) {

	ResourceVersionList := make([]string, len(targetPodList))

	for count := 0; count < len(targetPodList); count++ {

		pods, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(targetPodList[count], metav1.GetOptions{})
		if err != nil {
			return ResourceVersionList, errors.Errorf("Unable to get the pod, err: %v", err)
		}
		ResourceVersionList[count] = pods.ResourceVersion

	}

	if len(ResourceVersionList) == 0 {
		return ResourceVersionList, errors.Errorf("Fail to get the resource version of application pods")
	}
	klog.Infof("The Resource Version list is: %v", ResourceVersionList)

	return ResourceVersionList, nil
}

// CheckLoadPercentage checks the load percentage on every replicas
func CheckLoadPercentage(loadPercentage []string, replicaCount int) error {
	// It will make sure that the replica have some load
	// It will fail if replica will be 0% of load
	for count := 0; count < len(loadPercentage); count++ {

		if loadPercentage[count] == "0%" || loadPercentage[count] == "" {
			return errors.Errorf("The Load distribution percentage failed, as its value is: '%v'", loadPercentage[count])
		}
	}
	log.Info("[Check]: Check each replica should have some load")
	if len(loadPercentage) != replicaCount {
		return errors.Errorf("Fail to get the load on every replica")
	}
	return nil
}

// GetLoadDistrubution will get the load distribution on all the replicas of the application pod in an array formats
func GetLoadDistrubution(experimentsDetails *experimentTypes.ExperimentDetails, targetPod string) ([]string, error) {
	var out bytes.Buffer
	var stderr bytes.Buffer
	var emptyArray []string
	cmd := exec.Command("sh", "-c", `kubectl exec -it `+targetPod+` -n `+experimentsDetails.AppNS+` nodetool status | tail -n +6 | head -n -1`)
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		klog.Infof(fmt.Sprint(err) + ": " + stderr.String())
		klog.Infof("Error: %v", err)
		return emptyArray, errors.Errorf("Fail to get load distribution")
	}
	response := fmt.Sprint(out.String())
	split := strings.Split(response, "\n")
	loadPercentage := split[:len(split)-1]

	return loadPercentage, nil
}

// ResourceVersionCheck compare the resource version for target pods before and after chaos
func ResourceVersionCheck(ResourceVersionBefore, ResourceVersionAfter []string) error {

	if len(ResourceVersionBefore) == 0 || len(ResourceVersionAfter) == 0 {
		for count := 0; count < len(ResourceVersionAfter); count++ {
			if ResourceVersionBefore[count] == ResourceVersionAfter[count] {
				return errors.Errorf(" Resource Version Check failed!")
			}
		}
	}
	log.Info("Resource version check passed")

	return nil
}

// DeleteLivenessDeployment deletes the livenes deployments and wait for its termination
func DeleteLivenessDeployment(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	deletePolicy := metav1.DeletePropagationForeground
	if err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS).Delete("cassandra-liveness-deploy", &metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		return errors.Errorf("Fail to delete liveness deployment, due to %v", err)
	}
	err := retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS).List(metav1.ListOptions{LabelSelector: "name=cassandra-liveness-deploy"})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Liveness deployment is not deleted yet, err: %v", err)
			}
			return nil
		})
	return err
}

// DeleteLivenessService deletes the liveness service and wait for its termination
func DeleteLivenessService(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	deletePolicy := metav1.DeletePropagationForeground
	if err := clients.KubeClient.CoreV1().Services(experimentsDetails.AppNS).Delete("cassandra-liveness-service", &metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		return errors.Errorf("Fail to delete liveness service, due to %v", err)
	}
	err := retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS).List(metav1.ListOptions{LabelSelector: "name=cassandra-liveness-service"})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Liveness deployment is not deleted yet, err: %v", err)
			}
			return nil
		})
	return err
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
			Name: "cassandra-liveness-deploy",
			Labels: map[string]string{
				"name": "cassandra-liveness-deploy",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrint32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "cassandra-liveness-deploy",
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": "cassandra-liveness-deploy",
					},
				},
				Spec: apiv1.PodSpec{
					Volumes: []apiv1.Volume{
						apiv1.Volume{
							Name: "status-volume",
							VolumeSource: apiv1.VolumeSource{
								EmptyDir: &apiv1.EmptyDirVolumeSource{},
							},
						},
					},
					Containers: []apiv1.Container{
						apiv1.Container{
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
								apiv1.EnvVar{
									Name:  "LIVENESS_PERIOD_SECONDS",
									Value: "10",
								},
								apiv1.EnvVar{
									Name:  "LIVENESS_TIMEOUT_SECONDS",
									Value: "10",
								},
								apiv1.EnvVar{
									Name:  "LIVENESS_RETRY_COUNT",
									Value: "10",
								},
								apiv1.EnvVar{
									Name:  "CASSANDRA_SVC_NAME",
									Value: experimentsDetails.CassandraServiceName,
								},
								apiv1.EnvVar{
									Name:  "REPLICATION_FACTOR",
									Value: experimentsDetails.KeySpaceReplicaFactor,
								},
								apiv1.EnvVar{
									Name:  "CASSANDRA_PORT",
									Value: string(experimentsDetails.CassandraPort),
								},
							},
							Resources: apiv1.ResourceRequirements{},
							VolumeMounts: []apiv1.VolumeMount{
								apiv1.VolumeMount{
									Name:      "status-volume",
									MountPath: "/var/tmp",
								},
							},
							ImagePullPolicy: apiv1.PullPolicy("Always"),
						},
						apiv1.Container{
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
								apiv1.ContainerPort{
									HostPort:      0,
									ContainerPort: int32(experimentsDetails.LivenessServicePort),
								},
							},
							Env: []apiv1.EnvVar{
								apiv1.EnvVar{
									Name:  "INIT_WAIT_SECONDS",
									Value: "10",
								},
								apiv1.EnvVar{
									Name:  "LIVENESS_SVC_PORT",
									Value: string(experimentsDetails.LivenessServicePort),
								},
							},
							Resources: apiv1.ResourceRequirements{},
							VolumeMounts: []apiv1.VolumeMount{
								apiv1.VolumeMount{
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
	_, err := clients.KubeClient.AppsV1().Deployments(experimentsDetails.AppNS).Create(liveness)
	if err != nil {
		return errors.Errorf("fail to create liveness deployment, due to {%v}", err)
	}
	klog.Info("Liveness Deployment Created successfully!")
	return nil

}

// CreateLivenessService will create Cassandra liveness service
func CreateLivenessService(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	// Create resource object
	livenessSvc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cassandra-liveness-service",
			Labels: map[string]string{
				"name": "cassandra-liveness-service",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name:     "liveness",
					Protocol: corev1.Protocol("TCP"),
					Port:     int32(experimentsDetails.LivenessServicePort),
					TargetPort: intstr.IntOrString{
						Type:   intstr.Type(0),
						IntVal: 0,
					},
					NodePort: 0,
				},
			},
			Selector: map[string]string{
				"name": "cassandra-liveness-deploy",
			},
			HealthCheckNodePort: 0,
		},
	}

	// Creating liveness service
	_, err := clients.KubeClient.CoreV1().Services(experimentsDetails.AppNS).Create(livenessSvc)
	if err != nil {
		return errors.Errorf("fail to create liveness service, due to {%v}", err)
	}
	log.Info("Liveness service created successfully!")

	return nil
}

func ptrint32(p int32) *int32 {
	return &p
}
