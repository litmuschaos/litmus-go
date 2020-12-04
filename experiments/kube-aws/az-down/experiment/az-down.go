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

// returns all AZ's instances are located in
func getAzRangeOfInstances([]*experimentTypes.InstanceDetails) ([]string) {
	// TODO check all instances and return list of zones instances are located in
	return []string{}
}

// return random valid az to target
func selectRandomAz(azlist []string) (string) {
	return azlist[rand.Intn(len(azlist))]
}

// chooses an az at random to target
func getAzToTarget(instances []*experimentTypes.InstanceDetails) (string) {

	availableAzsToTarget := getAzRangeOfInstances(instances) // look at targeting more than 1 az if possible

	// we want to randomly target an AZ
	return selectRandomAz(availableAzsToTarget)
}

func getNetworkAclAssociationIdForSubnet(subnetId string) (string, error) {
	return "", nil //TODO implement
}

func getNetworkAclIdForSubnet (subnetId string) (string, error) {
	return "", nil //TODO implement
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

	// Initialise the probe details. Bail out upon error, as we haven't entered exp business logic yet
	if err := probe.InitializeProbesInChaosResultDetails(&chaosDetails, clients, &resultDetails); err != nil {
		log.Fatalf("Unable to initialise probes details from chaosengine, err: %v", err)
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
	// TODO create replicated topic in kafka

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
	clusterInstances, err := aws.GetClusterInstancesInRegion(&ec2Svc, experimentsDetails.ClusterIdentifier)
	if err != nil {
		log.Errorf("failed to get the ec2 instance instances in region, err: %v", err)
		failStep := "Getting all instances in region (pre-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	// select az to be targeted
	azToTarget := getAzToTarget(clusterInstances)

	// get subnets for target az
	azName := fmt.Sprintf("ZoneName: %s", azToTarget)
	azNameFilter := ec2.Filter{Values: []*string{&azName}}
	filters := []*ec2.Filter{&azNameFilter}

	subnets, err := ec2Svc.DescribeSubnets(&ec2.DescribeSubnetsInput{Filters: filters})

	// get vpcids for all subnets in az
	vpcIds := []*string{}
	for _, subnet := range subnets.Subnets {
		vpcIds = append(vpcIds, subnet.VpcId)
	}

	// get the subnet ids
	subnetIds := []*string{}
	for _, subnet := range subnets.Subnets {
		subnetIds = append(subnetIds, subnet.SubnetId)
	}

	// create and assign dummy acl to each vpc
	dummyAclIds := []string{}
	for _, vpcId := range vpcIds {
		output, err := ec2Svc.CreateNetworkAcl(&ec2.CreateNetworkAclInput{VpcId: vpcId})
		if err != nil {
			log.Errorf("Failed to assign dummy ACM to vpc of az being targeted, err: %v", err)
			failStep := "Failed to assign dummy ACL to vpc of az being targeted"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
		dummyAclIds = append(dummyAclIds, fmt.Sprint(output.NetworkAcl.NetworkAclId))
		log.Info(fmt.Sprintf("New ACL created and assigned to vpc id: %s.\n%s", vpcId, output))
	}

	// get NetworkAclAssociationId and NetworkAclId of subnets in az
	subnetAclDetails := map[string]map[string]string{}
	for _, subnet := range subnets.Subnets {
		networkAclAssociationId, err := getNetworkAclAssociationIdForSubnet(*subnet.SubnetId)
		networkAclId, err := getNetworkAclIdForSubnet(*subnet.SubnetId)
		subnetAclDetails[*subnet.SubnetId] = map[string]string{
			"NetworkAclAssociationId": networkAclAssociationId,
			"NetworkAclId": networkAclId,
		}
	}


	// associate dummy ACL with az subnets
	// subnet-id
	// network-acl-id to revert change

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

	// TODO - clean up after experiment

	// reassociate previous ACL
	// delete dummy ACL
}

