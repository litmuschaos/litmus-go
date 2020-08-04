package cassandra

import (
	"bytes"
	"fmt"
	"math/rand"
	"os/exec"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog"
)

// NodeToolStatusCheck checks for the distrubution of the load on the ring
func NodeToolStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	var err error
	var replicaCount int
	log.Info("[NodeToolStatus]: Get a random application pod name")

	// Getting random application pod and total replica count
	experimentsDetails.AppName, replicaCount, err = GetApplicationPod(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the application name and application nodename due to, err: %v", err)
	}
	log.Info("[Check]: Checking for the distribution of load on the ring")

	// Get the load percentage on the application pod
	loadPercentage, err := GetLoadDistrubution(experimentsDetails)
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
func LivenessCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	// Creating liveness deployment
	err := CreateLivenessPod(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("[Liveness]: Liveness check failed, due to %v", err)
	}

	// Creating liveness service
	err = CreateLivenessService(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("[Liveness]: Liveness Service Check failed, due to %v", err)
	}

	// Checking the status of liveness deployment pod
	log.Info("[Status]: Checking the status of the cassandra liveness pod")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, "name=cassandra-liveness-deploy", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("[Liveness]: Liveness pod is not in running state, err: %v", err)
	}

	// Get liveness pod name
	log.Info("[Liveness]: Fetch the cassandra-liveness pod name")
	experimentsDetails.AppName, _, err = GetApplicationPod(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the application name and application nodename due to, err: %v", err)
	}

	// Record cassandra liveness pod resource version
	log.Info("[Liveness]: Record the resource version of liveness pod")
	experimentsDetails.PodResourceVersion = GetPodResourceVersion(experimentsDetails, clients)
	if experimentsDetails.PodResourceVersion == "" {
		return errors.Errorf("Fail to get the pod resource version")
	}
	log.Infof("Resource Version before chaos of liveness pod is: %v", experimentsDetails.PodResourceVersion)

	return nil
}

// LivenessCleanup will check the status of liveness pod cycle and wait till the cycle comes to the complete state
// At last it removes/cleanup the liveness deploy and svc
func LivenessCleanup(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	// log.Info("[CleanUP]: Getting ClusterIP of liveness service")
	// ClusterIP, err := GetServiceClusterIP(experimentsDetails, clients)
	// if err != nil {
	// 	return errors.Errorf("Fail to get the CLusterIP of liveness service, due to {%v}", err)
	// }
	// port := strconv.Itoa(experimentsDetails.LivenessServicePort)
	// URL := "http://" + ClusterIP + ":" + port + ""
	// fmt.Println(URL)
	// response, err := http.Get(URL)
	// if err != nil {
	// 	return errors.Errorf("The HTTP request failed with error %s\n", err)
	// }
	// data, _ := ioutil.ReadAll(response.Body)
	// fmt.Println(string(data))

	// Record cassandra liveness pod resource version after chaos
	log.Info("[Liveness]: Record the resource version of liveness pod (after-chaos)")
	ResourceVersionAfter := GetPodResourceVersion(experimentsDetails, clients)
	if ResourceVersionAfter == "" {
		return errors.Errorf("Fail to get the pod resource version")
	}
	log.Infof("Resource Version after chaos of liveness pod is: %v", ResourceVersionAfter)
	if experimentsDetails.PodResourceVersion == ResourceVersionAfter {
		return errors.Errorf("[Check]: Resource Version Check failed")
	}
	log.Info("[Check]: Resource Version Check passed!")

	return nil
}

//GetApplicationPod will select a random replica of the application pod for chaos
func GetApplicationPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, int, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(podList.Items) == 0 {
		return "", 0, errors.Errorf("Fail to get the application pod in %v namespace", experimentsDetails.AppNS)
	}

	rand.Seed(time.Now().Unix())
	randomIndex := rand.Intn(len(podList.Items))
	applicationName := podList.Items[randomIndex].Name
	return applicationName, len(podList.Items), nil
}

// GetPodResourceVersion will return the resource version of the application pod
func GetPodResourceVersion(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) string {
	pod, _ := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(experimentsDetails.AppName, metav1.GetOptions{})
	return pod.ResourceVersion
}

// GetServiceClusterIP will return the cluster IP of the liveness service
func GetServiceClusterIP(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {

	serviceList, _ := clients.KubeClient.CoreV1().Services(experimentsDetails.AppNS).Get("cassandra-liveness-service", metav1.GetOptions{})
	if len(serviceList.Name) == 0 {
		return "", errors.Errorf("Fail to get the liveness service")
	}
	return serviceList.Spec.ClusterIP, nil
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
func GetLoadDistrubution(experimentsDetails *experimentTypes.ExperimentDetails) ([]string, error) {
	var out bytes.Buffer
	var stderr bytes.Buffer
	var emptyArray []string
	cmd := exec.Command("sh", "-c", `kubectl exec -it `+experimentsDetails.AppName+`-n `+experimentsDetails.AppNS+`nodetool status | awk '{print $6}' | tail -n +6 | head -n -1`)
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
		panic(err)
	}
	log.Info("Liveness service created successfully!")

	return nil
}

func ptrint32(p int32) *int32 {
	return &p
}
