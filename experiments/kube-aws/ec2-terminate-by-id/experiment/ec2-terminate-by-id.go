package experiment

import (
	"os"
	"strings"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/ec2-terminate-by-id/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	aws "github.com/litmuschaos/litmus-go/pkg/cloud/aws/ec2"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/kube-aws/ec2-terminate-by-id/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ec2-terminate-by-id/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
)

// EC2TerminateByID inject the ebs volume loss chaos
func EC2TerminateByID(clients clients.ClientSets) {

	var (
		err                  error
		activeNodeCount      int
		autoScalingGroupName string
	)
	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}

	//Fetching all the ENV passed from the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", os.Getenv("EXPERIMENT_NAME"))
	experimentEnv.GetENV(&experimentsDetails)

	// Initialize the chaos attributes
	types.InitialiseChaosVariables(&chaosDetails)

	// Initialize Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	if experimentsDetails.EngineName != "" {
		// Initialize the probe details. Bail out upon error, as we haven't entered exp business logic yet
		if err = probe.InitializeProbesInChaosResultDetails(&chaosDetails, clients, &resultDetails); err != nil {
			log.Errorf("Unable to initialize the probes, err: %v", err)
			return
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	if err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT"); err != nil {
		log.Errorf("Unable to Create the Chaos Result, err: %v", err)
		failStep := "[pre-chaos]: Failed to update the chaos result of ec2 terminate experiment (SOT), err: " + err.Error()
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	// generating the event in chaosresult to marked the verdict as awaited
	msg := "experiment: " + experimentsDetails.ExperimentName + ", Result: Awaited"
	types.SetResultEventAttributes(&eventsDetails, types.AwaitedVerdict, msg, "Normal", &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	//DISPLAY THE INSTANCE INFORMATION
	log.InfoWithValues("The instance information is as follows", logrus.Fields{
		"Chaos Duration":  experimentsDetails.ChaosDuration,
		"Chaos Namespace": experimentsDetails.ChaosNamespace,
		"Instance ID":     experimentsDetails.Ec2InstanceID,
		"Sequence":        experimentsDetails.Sequence,
	})

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go common.AbortWatcherWithoutExit(experimentsDetails.ExperimentName, clients, &resultDetails, &chaosDetails, &eventsDetails)

	//PRE-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	if err = status.AUTStatusCheck(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.TargetContainer, experimentsDetails.Timeout, experimentsDetails.Delay, clients, &chaosDetails); err != nil {
		log.Errorf("Application status check failed, err: %v", err)
		failStep := "[pre-chaos]: Failed to verify that the AUT (Application Under Test) is in running state, err: " + err.Error()
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	//PRE-CHAOS AUXILIARY APPLICATION STATUS CHECK
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running (pre-chaos)")
		if err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			log.Errorf("Auxiliary Application status check failed, err: %v", err)
			failStep := "[pre-chaos]: Failed to verify that the Auxiliary Applications are in running state, err: " + err.Error()
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
	}

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the pre-chaos check
		if len(resultDetails.ProbeDetails) != 0 {

			if err = probe.RunProbes(&chaosDetails, clients, &resultDetails, "PreChaos", &eventsDetails); err != nil {
				log.Errorf("Probe Failed, err: %v", err)
				failStep := "[pre-chaos]: Failed while running probes, err: " + err.Error()
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

	//Verify the aws ec2 instance is running (pre chaos)
	if err = aws.InstanceStatusCheckByID(experimentsDetails.Ec2InstanceID, experimentsDetails.Region); err != nil {
		log.Errorf("failed to get the ec2 instance status, err: %v", err)
		failStep := "[pre-chaos]: Failed to verify the AWS ec2 instance status, err: " + err.Error()
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}
	log.Info("[Status]: EC2 instance is in running state")

	//PRE-CHAOS NODE STATUS CHECK
	if experimentsDetails.ManagedNodegroup == "enable" {
		activeNodeCount, autoScalingGroupName, err = aws.PreChaosNodeCountCheck(strings.Split(experimentsDetails.Ec2InstanceID, ","), experimentsDetails.Region)
		if err != nil {
			log.Errorf("Pre chaos node status check failed, err: %v", err)
			failStep := "[pre-chaos]: Failed to verify that the NUT (Node Under Test) is running (pre-chaos), err: " + err.Error()
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
	}

	// Including the litmus lib for ec2-terminate
	switch experimentsDetails.ChaosLib {
	case "litmus":
		if err = litmusLIB.PrepareEC2TerminateByID(&experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails); err != nil {
			log.Errorf("Chaos injection failed, err: %v", err)
			failStep := "[chaos]: Failed inside the chaoslib, err: " + err.Error()
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
	default:
		log.Error("[Invalid]: Please Provide the correct LIB")
		failStep := "[chaos]: no match was found for the specified lib"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	log.Infof("[Confirmation]: %v chaos has been injected successfully", experimentsDetails.ExperimentName)
	resultDetails.Verdict = v1alpha1.ResultVerdictPassed

	// POST-CHAOS ACTIVE NODE COUNT TEST
	if experimentsDetails.ManagedNodegroup == "enable" {
		if err := aws.PostChaosNodeCountCheck(activeNodeCount, autoScalingGroupName, experimentsDetails.Region); err != nil {
			log.Errorf("Post chaos active node count check failed, err: %v", err)
			failStep := "[post-chaos]: Failed to verify the active number of nodes, err: " + err.Error()
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
	}

	//Verify the aws ec2 instance is running (post chaos)
	if experimentsDetails.ManagedNodegroup != "enable" {
		if err = aws.InstanceStatusCheckByID(experimentsDetails.Ec2InstanceID, experimentsDetails.Region); err != nil {
			log.Errorf("failed to get the ec2 instance status, err: %v", err)
			failStep := "[post-chaos]: Failed to verify the AWS ec2 instance status, err: " + err.Error()
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
		log.Info("[Status]: EC2 instance is in running state (post chaos)")
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	if err = status.AUTStatusCheck(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.TargetContainer, experimentsDetails.Timeout, experimentsDetails.Delay, clients, &chaosDetails); err != nil {
		log.Errorf("Application status check failed, err: %v", err)
		failStep := "[post-chaos]: Failed to verify that the AUT (Application Under Test) is running, err: " + err.Error()
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	//POST-CHAOS AUXILIARY APPLICATION STATUS CHECK
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running (post-chaos)")
		if err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			log.Errorf("Auxiliary Application status check failed, err: %v", err)
			failStep := "[post-chaos]: Failed to verify that the Auxiliary Applications are running, err: " + err.Error()
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
	}

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			if err = probe.RunProbes(&chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails); err != nil {
				log.Errorf("Probes Failed, err: %v", err)
				failStep := "[post-chaos]: Failed while running probes, err: " + err.Error()
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
	if err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT"); err != nil {
		log.Errorf("Unable to Update the Chaos Result, err:  %v", err)
		return
	}

	// generating the event in chaosresult to marked the verdict as pass/fail
	msg = "experiment: " + experimentsDetails.ExperimentName + ", Result: " + string(resultDetails.Verdict)
	reason := types.PassVerdict
	eventType := "Normal"
	if resultDetails.Verdict != "Pass" {
		reason = types.FailVerdict
		eventType = "Warning"
	}
	types.SetResultEventAttributes(&eventsDetails, reason, msg, eventType, &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	if experimentsDetails.EngineName != "" {
		msg := experimentsDetails.ExperimentName + " experiment has been " + string(resultDetails.Verdict) + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

}
