package experiment

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/ec2"
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/az-down/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/kube-aws/az-down/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/az-down/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
	"math/rand"
)

// returns all AZs instances are located in
func getAzRangeOfInstances(instances []*experimentTypes.InstanceDetails) []string {

	// filter unique az values
	uniqueAzs := make(map[string]string)
	for _, instance := range instances {
		uniqueAzs[instance.AZ] = ""
	}

	azNames := make([]string, 0, len(uniqueAzs))
	for azName, _ := range uniqueAzs {
		azNames = append(azNames, azName)
	}

	return azNames
}

// return random valid az to target
func selectRandomAz(azlist []string) string {
	return azlist[rand.Intn(len(azlist))]
}

// chooses an az at random to target
func getAzToTarget(instances []*experimentTypes.InstanceDetails) string {

	availableAzsToTarget := getAzRangeOfInstances(instances) // look at targeting more than 1 az if possible

	// we want to randomly target an AZ
	return selectRandomAz(availableAzsToTarget)
}

// returns NetworkAclId and NetworkAclAssociationId of an ACL for a given subnet id
func getNetworkAclDetails(ec2Svc *ec2.EC2, subnetId string) (experimentTypes.Subnet, error) {

	filterName := "association.subnet-id"
	subnetFilter := ec2.Filter{
		Name:   &filterName,
		Values: []*string{&subnetId},
	}
	filters := []*ec2.Filter{&subnetFilter}

	aclOutput, err := ec2Svc.DescribeNetworkAcls(&ec2.DescribeNetworkAclsInput{Filters: filters})

	if err != nil {
		return experimentTypes.Subnet{}, err
	}

	subnetAclDetails := experimentTypes.Subnet{
		NetworkAclId:            *aclOutput.NetworkAcls[0].Associations[0].NetworkAclId,
		NetworkAclAssociationId: *aclOutput.NetworkAcls[0].Associations[0].NetworkAclAssociationId,
	}

	return subnetAclDetails, nil
}

func AZDown(clients clients.ClientSets) {

	var err error
	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}

	//Fetching all the ENV passed from the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", experimentsDetails.ExperimentName)
	experimentEnv.GetENV(&experimentsDetails)

	// Initialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Initialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	if experimentsDetails.EngineName != "" {
		// Initialise the probe details. Bail out upon error, as we haven't entered exp business logic yet
		if err := probe.InitializeProbesInChaosResultDetails(&chaosDetails, clients, &resultDetails); err != nil {
			log.Fatalf("Unable to initialise probes details from chaosengine, err: %v", err)
		}
	}


	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result, err: %v", err)
		failStep := "Updating the chaos result of az-down experiment (SOT)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	// generating the event in chaosresult to marked the verdict as awaited
	msg := "experiment: " + experimentsDetails.ExperimentName + ", Result: Awaited"
	types.SetResultEventAttributes(&eventsDetails, types.AwaitedVerdict, msg, "Normal", &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("The application information is as follows", logrus.Fields{
		"Namespace": experimentsDetails.AppNS,
		"Label":     experimentsDetails.AppLabel,
		"Ramp Time": experimentsDetails.RampTime,
	})

	//PRE-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		log.Errorf("Application status check failed, err: %v", err)
		failStep := "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the pre-chaos check
		if len(resultDetails.ProbeDetails) != 0 {

			err = probe.RunProbes(&chaosDetails, clients, &resultDetails, "PreChaos", &eventsDetails)
			if err != nil {
				log.Errorf("Probe Failed, err: %v", err)
				failStep := "Failed while running probes"
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Warning", &chaosDetails)
				events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
				result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}
		// generating the events for the pre-chaos check
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// start of experiment logic

	// Configure AWS Credentials
	if err = aws.ConfigureAWS(); err != nil {
		log.Errorf("AWS authentication failed, err: %v", err)
		failStep := "Configure AWS configuration (pre-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	// create ec2 client
	ec2Svc := aws.GetNewEC2Client(&experimentsDetails)

	// Get all instances in region that belong to our cluster under test
	clusterInstances, err := aws.GetClusterInstancesInRegion(&ec2Svc, &experimentsDetails)
	if err != nil {
		log.Errorf("failed to get the ec2 instances in region, err: %v", err)
		failStep := "Getting all instances in region (pre-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}
	log.Info(fmt.Sprintf("Instances that match cluster-identifier %s: %s", experimentsDetails.ClusterIdentifier, clusterInstances))

	// return early if no instances match cluster identifier
	if len(clusterInstances) < 1 {
		log.Errorf("Unable to find any ec2 instances in region that match cluster identifier, err: %v", err)
		failStep := "Unable to find any ec2 instances in region that match cluster identifier (pre-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	// select az to be targeted
	azToTarget := getAzToTarget(clusterInstances)
	log.Info(fmt.Sprintf("Targeting cluster instances in az %s", azToTarget))

	// get subnets for target az
	filterName := "availabilityZone"
	azNameFilter := ec2.Filter{
		Name:   &filterName,
		Values: []*string{&azToTarget},
	}
	filters := []*ec2.Filter{&azNameFilter}

	subnetsDescription, err := ec2Svc.DescribeSubnets(&ec2.DescribeSubnetsInput{Filters: filters})

	// gather details of subnets in target az
	azDetails := experimentTypes.AzDetails{
		AzName: azToTarget,
	}
	subnetDetails := []*experimentTypes.Subnet{}
	for _, subnet := range subnetsDescription.Subnets {
		subnetDetail := experimentTypes.Subnet{
			Id:    *subnet.SubnetId,
			VpcId: *subnet.VpcId,
		}
		subnetDetails = append(subnetDetails, &subnetDetail)
		log.Info(fmt.Sprintf("Identified subnet in target az %s: SubnetId:%s VpcId:%s", azToTarget, subnet.SubnetId, subnet.VpcId))
	}
	azDetails.Subnets = subnetDetails
	log.Info(fmt.Sprintf("Details of instances in az %s being taken down: %s", azToTarget, azDetails))

	// create and assign dummy acl to each subnet vpc
	vpcDummyAcls := make(map[string]string)
	for i, subnet := range azDetails.Subnets {
		// create only if required, otherwise associate vpc with pre-existing dummy ACL
		_, ok := vpcDummyAcls[subnet.VpcId]
		if !ok {
			output, err := ec2Svc.CreateNetworkAcl(&ec2.CreateNetworkAclInput{DryRun: &experimentsDetails.DryRun, VpcId: &subnet.VpcId})
			if err != nil {
				log.Errorf("Failed to assign dummy ACM to vpc of az being targeted, err: %v", err)
				failStep := "Failed to assign dummy ACL to vpc of az being targeted"
				result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
				return
			}
			dummyAclId := fmt.Sprint(*output.NetworkAcl.NetworkAclId)
			subnet.DummyAclId = dummyAclId
			vpcDummyAcls[subnet.VpcId] = dummyAclId
			log.Info(fmt.Sprintf("Dummy ACL id:%s created for vpc id: %s.", dummyAclId, subnet.VpcId))
		}

		subnet.DummyAclId = vpcDummyAcls[subnet.VpcId]
		azDetails.Subnets[i] = subnet
		log.Info(fmt.Sprintf("Dummy ACL id:%s associated with vpc id: %s.", subnet.DummyAclId, subnet.VpcId))
	}

	// get NetworkAclAssociationId and NetworkAclId of subnets in az
	for _, subnet := range azDetails.Subnets {
		networkAclDetails, err := getNetworkAclDetails(&ec2Svc, subnet.Id)
		if err != nil {
			log.Errorf("Failed to get NetworkAclAssociationId for subnet %s, err: %v", subnet.Id, err)
			failStep := fmt.Sprintf("Failed to get NetworkAclAssociationId for subnet %s in az being targeted", subnet.Id)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
		subnet.NetworkAclAssociationId = networkAclDetails.NetworkAclAssociationId
		subnet.NetworkAclId = networkAclDetails.NetworkAclId
		log.Info(fmt.Sprintf("ACL details of subnet %s pre-chaos. NetworkAclAssociationId: %s, NetworkAclId:%s", subnet.Id, subnet.NetworkAclId, subnet.NetworkAclId))
	}

	// replace old acl with new acl to take nodes in az out of circulation
	aclAssociations := make(map[string]string)
	for _, subnet := range azDetails.Subnets {
		_, ok := aclAssociations[subnet.NetworkAclId]
		// replace association with dummy acl if not replaced already
		if !ok {
			_, err := ec2Svc.ReplaceNetworkAclAssociation(&ec2.ReplaceNetworkAclAssociationInput{
				DryRun: &experimentsDetails.DryRun,
				AssociationId: &subnet.NetworkAclAssociationId,
				NetworkAclId:  &subnet.DummyAclId,
			})
			if err != nil {
				log.Errorf("Failed to replace association-id:%s ACL:%s with dummy ACL:%s, err: %v", subnet.NetworkAclAssociationId, subnet.NetworkAclId, subnet.DummyAclId, err)
				failStep := fmt.Sprintf("Failed to replace association-id:%s ACL:%s with dummy ACL:%s", subnet.NetworkAclAssociationId, subnet.NetworkAclId, subnet.DummyAclId)
				result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
				return
			}
			aclAssociations[subnet.NetworkAclId] = subnet.NetworkAclAssociationId
			log.Info(fmt.Sprintf("Replaced NetworkAclAssociationId:%s NetworkAclId:%s with dummy DummyAclId:%s", subnet.NetworkAclAssociationId, subnet.NetworkAclId, subnet.DummyAclId))
		}
	}

	// Including the litmus lib
	if experimentsDetails.ChaosLib == "litmus" {
		err = litmusLIB.AZDown()
		if err != nil {
			log.Errorf("Chaos injection failed, err: %v", err)
			failStep := "failed in chaos injection phase"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
		log.Info("[Confirmation]: chaos has been injected successfully")
		resultDetails.Verdict = "Pass"
	} else {
		log.Error("[Invalid]: Please Provide the correct LIB")
		failStep := "no match found for specified lib"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		log.Errorf("Application status check failed, err: %v", err)
		failStep := "Verify that the AUT (Application Under Test) is running (post-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			err = probe.RunProbes(&chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails)
			if err != nil {
				log.Errorf("Probes Failed, err: %v", err)
				failStep := "Failed while running probes"
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Warning", &chaosDetails)
				events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
				result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}

		// generating post chaos event
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	//Updating the chaosResult in the end of experiment
	log.Infof("[The End]: Updating the chaos result of %v experiment (EOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
	if err != nil {
		log.Fatalf("Unable to Update the Chaos Result, err: %v", err)
	}

	// generating the event in chaosresult to marked the verdict as pass/fail
	msg = "experiment: " + experimentsDetails.ExperimentName + ", Result: " + resultDetails.Verdict
	reason := types.PassVerdict
	eventType := "Normal"
	if resultDetails.Verdict != "Pass" {
		reason = types.FailVerdict
		eventType = "Warning"
	}
	types.SetResultEventAttributes(&eventsDetails, reason, msg, eventType, &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	if experimentsDetails.EngineName != "" {
		msg := experimentsDetails.ExperimentName + " experiment has been " + resultDetails.Verdict + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// cleanup: re-associate pre-chaos ACL to bring nodes in target az back into circulation
	for _, subnet := range azDetails.Subnets {
		_, err := ec2Svc.ReplaceNetworkAclAssociation(&ec2.ReplaceNetworkAclAssociationInput{
			DryRun: &experimentsDetails.DryRun,
			AssociationId: &subnet.NetworkAclAssociationId,
			NetworkAclId:  &subnet.NetworkAclId,
		})
		if err != nil {
			log.Errorf("Failed to revert association-id:%s dummy-ACL:%s to original ACL:%s, err: %v", subnet.NetworkAclAssociationId, subnet.DummyAclId, subnet.NetworkAclId, err)
			failStep := fmt.Sprintf("Failed to revert association-id:%s dummy-ACL:%s to original ACL:%s", subnet.NetworkAclAssociationId, subnet.DummyAclId, subnet.NetworkAclId)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
		log.Info(fmt.Sprintf("Reverted NetworkAclAssociationId:%s DummyAclId:%s to original NetworkAclId:%s", subnet.NetworkAclAssociationId, subnet.DummyAclId, subnet.NetworkAclId))
	}

	// cleanup: remove dummy ACLs no longer needed
	for _, subnet := range azDetails.Subnets {
		_, err := ec2Svc.DeleteNetworkAcl(&ec2.DeleteNetworkAclInput{
			DryRun: &experimentsDetails.DryRun,
			NetworkAclId: &subnet.DummyAclId,
		})
		if err != nil {
			log.Errorf("Failed to remove dummy ACL %s, err: %v", subnet.DummyAclId, err)
			failStep := fmt.Sprintf("Failed to revert dummy ACL %s (post-chaos)", subnet.DummyAclId)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
		log.Info(fmt.Sprintf("DummyAclId:%s removed", subnet.DummyAclId))
	}
}
