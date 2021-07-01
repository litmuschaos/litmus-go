package gcp

import (
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetVMInstanceStatus returns the status of a VM instance
func GetVMInstanceStatus(instanceName string, gcpProjectID string, instanceZone string) (string, error) {
	var err error

	// create an empty context
	ctx := context.Background()

	// get service account credentials json
	json, err := GetServiceAccountJSONFromSecret()
	if err != nil {
		return "", err
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return "", err
	}

	// get information about the requisite VM instance
	response, err := computeService.Instances.Get(gcpProjectID, instanceZone, instanceName).Context(ctx).Do()
	if err != nil {
		return "", err
	}

	// return the VM status
	return response.Status, nil
}

// PreChaosNodeStatusCheck fetches all the nodes in the cluster and checks their status, and fetches the total active nodes in the cluster, prior to the chaos experiment
func PreChaosNodeStatusCheck(timeout, delay int, clients clients.ClientSets) (int, error) {
	nodeList, err := clients.KubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return 0, errors.Errorf("fail to get the nodes, err: %v", err)
	}
	for _, node := range nodeList.Items {
		if err = status.CheckNodeStatus(node.Name, timeout, delay, clients); err != nil {
			log.Infof("[Info]: The cluster is unhealthy this might not work, due to %v", err)
		}
	}
	activeNodeCount, err := getActiveNodeCount(clients)
	if err != nil {
		return 0, errors.Errorf("fail to get the total active node count pre chaos, err: %v", err)
	}

	return activeNodeCount, nil
}

// PostChaosActiveNodeCountCheck checks the number of active nodes post chaos and validates the number of healthy node count post chaos
func PostChaosActiveNodeCountCheck(activeNodeCount, timeout, delay int, clients clients.ClientSets) error {
	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			activeNodes, err := getActiveNodeCount(clients)
			if err != nil {
				return errors.Errorf("fail to get the total active nodes, err: %v", err)
			}
			if activeNodeCount != activeNodes {
				return errors.Errorf("fail to get equal active node post chaos")
			}
			return nil
		})
	return err
}

// getActiveNodeCount fetches the target node and total node count from the cluster
func getActiveNodeCount(clients clients.ClientSets) (int, error) {
	nodeList, err := clients.KubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return 0, errors.Errorf("fail to get the nodes, err: %v", err)
	}

	nodeCount := 0
	for _, node := range nodeList.Items {

		conditions := node.Status.Conditions
		for _, condition := range conditions {
			if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
				nodeCount++
			}
		}
	}
	log.Infof("[Info]: Total number active nodes are: %v", nodeCount)

	return nodeCount, nil
}

//InstanceStatusCheckByName is used to check the status of all the VM instances under chaos
func InstanceStatusCheckByName(instanceNames string, gcpProjectId string, instanceZones string) error {
	instanceNamesList := strings.Split(instanceNames, ",")
	instanceZonesList := strings.Split(instanceZones, ",")
	if len(instanceNamesList) == 0 {
		return errors.Errorf("No instance name found to stop")
	}
	if len(instanceNamesList) != len(instanceZonesList) {
		return errors.Errorf("The number of instance names and the number of regions is not equal")
	}
	log.Infof("[Info]: The instances under chaos(IUC) are: %v", instanceNamesList)
	return InstanceStatusCheck(instanceNamesList, gcpProjectId, instanceZonesList)
}

//InstanceStatusCheck is used to check whether all VM instances under chaos are running or not
func InstanceStatusCheck(instanceNamesList []string, gcpProjectId string, instanceZonesList []string) error {
	for i := range instanceNamesList {
		instanceState, err := GetVMInstanceStatus(instanceNamesList[i], gcpProjectId, instanceZonesList[i])
		if err != nil {
			return err
		}
		if instanceState != "RUNNING" {
			return errors.Errorf("failed to get the vm instance '%v' in running sate, current state: %v", instanceNamesList[i], instanceState)
		}
	}
	return nil
}
