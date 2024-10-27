package experiment

import (
	"context"
	"os"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/ec2-terminate-by-tag/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	aws "github.com/litmuschaos/litmus-go/pkg/cloud/aws/ec2"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/kube-aws/ec2-terminate-by-tag/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ec2-terminate-by-tag/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// EC2TerminateByTag inject the ebs volume loss chaos
func EC2TerminateByTag(ctx context.Context, clients clients.ClientSets) {
	span := trace.SpanFromContext(ctx)

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
		// Get values from chaosengine. Bail out upon error, as we haven't entered exp business logic yet
		if err = types.GetValuesFromChaosEngine(&chaosDetails, clients, &resultDetails); err != nil {
			log.Errorf("Unable to initialize the probes: %v", err)
			span.SetStatus(codes.Error, "Unable to initialize the probes")
			span.RecordError(err)
			return
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	if err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT"); err != nil {
		log.Errorf("Unable to create the chaosresult: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Unable to create the chaosresult")
		span.RecordError(err)
		return
	}

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	// generating the event in chaosresult to marked the verdict as awaited
	msg := "experiment: " + experimentsDetails.ExperimentName + ", Result: Awaited"
	types.SetResultEventAttributes(&eventsDetails, types.AwaitedVerdict, msg, "Normal", &resultDetails)
	if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult"); eventErr != nil {
		log.Errorf("Failed to create %v event inside chaosresult", types.AwaitedVerdict)
	}

	//DISPLAY THE INSTANCE INFORMATION
	log.InfoWithValues("The instance information is as follows", logrus.Fields{
		"Chaos Duration":               experimentsDetails.ChaosDuration,
		"Chaos Namespace":              experimentsDetails.ChaosNamespace,
		"Instance Tag":                 experimentsDetails.Ec2InstanceTag,
		"Instance Affected Percentage": experimentsDetails.InstanceAffectedPerc,
		"Sequence":                     experimentsDetails.Sequence,
	})

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go common.AbortWatcherWithoutExit(experimentsDetails.ExperimentName, clients, &resultDetails, &chaosDetails, &eventsDetails)

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the pre-chaos check
		if len(resultDetails.ProbeDetails) != 0 {

			if err = probe.RunProbes(ctx, &chaosDetails, clients, &resultDetails, "PreChaos", &eventsDetails); err != nil {
				log.Errorf("Probe Failed: %v", err)
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Warning", &chaosDetails)
				if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
					log.Errorf("Failed to create %v event inside chaosengine", types.PreChaosCheck)
				}
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
				span.SetStatus(codes.Error, "Probe Failed")
				span.RecordError(err)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}
		// generating the events for the pre-chaos check
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Normal", &chaosDetails)
		if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
			log.Errorf("Failed to create %v event inside chaosengine", types.PreChaosCheck)
		}
	}

	//selecting the target instance (pre chaos)
	if err = litmusLIB.SetTargetInstance(&experimentsDetails); err != nil {
		log.Errorf("Failed to get the target ec2 instance: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Failed to get the target ec2 instance")
		span.RecordError(err)
		return
	}

	//PRE-CHAOS NODE STATUS CHECK
	if experimentsDetails.ManagedNodegroup == "enable" {
		log.Info("[Status]: Counting number of active nodes in the node group (pre-chaos)")
		activeNodeCount, autoScalingGroupName, err = aws.PreChaosNodeCountCheck(experimentsDetails.TargetInstanceIDList, experimentsDetails.Region)
		if err != nil {
			log.Errorf("Pre chaos node status check failed: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "Pre chaos node status check failed")
			span.RecordError(err)
			return
		}
	}

	chaosDetails.Phase = types.ChaosInjectPhase

	if err = litmusLIB.PrepareEC2TerminateByTag(ctx, &experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails); err != nil {
		log.Errorf("Chaos injection failed: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Chaos injection failed")
		span.RecordError(err)
		return
	}

	log.Infof("[Confirmation]: %v chaos has been injected successfully", experimentsDetails.ExperimentName)
	resultDetails.Verdict = v1alpha1.ResultVerdictPassed

	chaosDetails.Phase = types.PostChaosPhase

	//Verify the aws ec2 instance is running (post chaos)
	if chaosDetails.DefaultHealthCheck && experimentsDetails.ManagedNodegroup != "enable" {
		log.Info("[Status]: Verify that the aws ec2 instances are in running state (post-chaos)")
		if err = aws.InstanceStatusCheck(experimentsDetails.TargetInstanceIDList, experimentsDetails.Region); err != nil {
			log.Errorf("Failed to get the ec2 instance status as running post chaos: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "Failed to get the ec2 instance status as running post chaos")
			span.RecordError(err)
			return
		}
		log.Info("[Status]: EC2 instance is in running state (post chaos)")
	}

	// POST-CHAOS ACTIVE NODE COUNT TEST
	if experimentsDetails.ManagedNodegroup == "enable" {
		log.Info("[Status]: Counting and verifying number of active nodes in the node group (post-chaos)")
		if err = aws.PostChaosNodeCountCheck(activeNodeCount, autoScalingGroupName, experimentsDetails.Region); err != nil {
			log.Errorf("Post chaos active node count check failed: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "Post chaos active node count check failed")
			span.RecordError(err)
			return
		}
	}
	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			if err = probe.RunProbes(ctx, &chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails); err != nil {
				log.Errorf("Probes Failed: %v", err)
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Warning", &chaosDetails)
				if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
					log.Errorf("Failed to create %v event inside chaosengine", types.PostChaosCheck)
				}
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
				span.SetStatus(codes.Error, "Probes Failed")
				span.RecordError(err)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}

		// generating post chaos event
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Normal", &chaosDetails)
		if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
			log.Errorf("Failed to create %v event inside chaosengine", types.PostChaosCheck)
		}
	}

	//Updating the chaosResult in the end of experiment
	log.Infof("[The End]: Updating the chaos result of %v experiment (EOT)", experimentsDetails.ExperimentName)
	if err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT"); err != nil {
		log.Errorf("Unable to update the chaosresult:  %v", err)
		span.SetStatus(codes.Error, "Unable to update the chaosresult")
		span.RecordError(err)
		return
	}

	// generating the event in chaosresult to marked the verdict as pass/fail
	msg = "experiment: " + experimentsDetails.ExperimentName + ", Result: " + string(resultDetails.Verdict)
	reason, eventType := types.GetChaosResultVerdictEvent(resultDetails.Verdict)
	types.SetResultEventAttributes(&eventsDetails, reason, msg, eventType, &resultDetails)
	if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult"); eventErr != nil {
		log.Errorf("Failed to create %v event inside chaosresult", reason)
	}

	if experimentsDetails.EngineName != "" {
		msg := experimentsDetails.ExperimentName + " experiment has been " + string(resultDetails.Verdict) + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
			log.Errorf("Failed to create %v event inside chaosengine", types.Summary)
		}
	}

}
