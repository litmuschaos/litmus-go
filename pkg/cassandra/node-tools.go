package cassandra

import (
	"strings"

	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/cassandra/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeToolStatusCheck checks for the distribution of the load on the ring
func NodeToolStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	var err error
	var replicaCount int

	// Getting application pod list
	targetPodName, err := GetApplicationPodName(experimentsDetails, clients)
	if err != nil {
		return err
	}
	log.Infof("[NodeToolStatus]: The application pod name for checking load distribution: %v", targetPodName)

	replicaCount, err = GetApplicationReplicaCount(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get app replica count, due to %v", err)
	}
	log.Info("[Check]: Checking for the distribution of load on the ring")

	// Get the load percentage on the application pod
	loadPercentage, err := GetLoadDistribution(experimentsDetails, clients, targetPodName)
	if err != nil {
		return errors.Errorf("Load distribution check failed, due to %v", err)
	}

	// Check the load precentage
	if err = CheckLoadPercentage(loadPercentage, replicaCount); err != nil {
		return errors.Errorf("Load percentage check failed, due to %v", err)
	}

	return nil
}

//GetApplicationPodName will return the name of first application pod
func GetApplicationPodName(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaoslibDetail.AppNS).List(metav1.ListOptions{LabelSelector: experimentsDetails.ChaoslibDetail.AppLabel})
	if err != nil || len(podList.Items) == 0 {
		return "", errors.Errorf("Fail to get the application pod in %v namespace", experimentsDetails.ChaoslibDetail.AppNS)
	}

	return podList.Items[0].Name, nil
}

//GetApplicationReplicaCount will return the replica count of the sts application
func GetApplicationReplicaCount(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (int, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaoslibDetail.AppNS).List(metav1.ListOptions{LabelSelector: experimentsDetails.ChaoslibDetail.AppLabel})
	if err != nil || len(podList.Items) == 0 {
		return 0, errors.Errorf("Fail to get the application pod in %v namespace", experimentsDetails.ChaoslibDetail.AppNS)
	}
	return len(podList.Items), nil
}

// CheckLoadPercentage checks the load percentage on every replicas
func CheckLoadPercentage(loadPercentage []string, replicaCount int) error {

	// It will make sure that the replica have some load
	// It will fail if replica has 0% load
	if len(loadPercentage) != replicaCount {
		return errors.Errorf("Fail to get the load on every replica")
	}

	for count := 0; count < len(loadPercentage); count++ {

		if loadPercentage[count] == "0%" || loadPercentage[count] == "" {
			return errors.Errorf("The Load distribution percentage failed, as its value is: '%v'", loadPercentage[count])
		}
	}
	log.Info("[Check]: Load is distributed over all the replica of cassandra")

	return nil
}

// GetLoadDistribution will get the load distribution on all the replicas of the application pod in an array formats
func GetLoadDistribution(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, targetPod string) ([]string, error) {

	// It will contains all the pod & container details required for exec command
	execCommandDetails := litmusexec.PodDetails{}

	command := append([]string{"/bin/sh", "-c"}, "nodetool status  | awk '{print $6}' | tail -n +6 | head -n -1")
	litmusexec.SetExecCommandAttributes(&execCommandDetails, targetPod, "cassandra", experimentsDetails.ChaoslibDetail.AppNS)
	response, err := litmusexec.Exec(&execCommandDetails, clients, command)
	if err != nil {
		return nil, errors.Errorf("Unable to get nodetool status details due to err: %v", err)
	}
	split := strings.Split(response, "\n")
	loadPercentage := split[:len(split)-1]

	return loadPercentage, nil
}
