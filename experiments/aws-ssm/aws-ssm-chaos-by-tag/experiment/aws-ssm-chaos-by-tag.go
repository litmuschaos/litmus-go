package experiment

import (
	"os"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/aws-ssm-chaos/lib/ssm"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/aws-ssm/aws-ssm-chaos/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/aws-ssm/aws-ssm-chaos/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	ec2 "github.com/litmuschaos/litmus-go/pkg/cloud/aws/ec2"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/ssm"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
)

// AWSSSMChaosByTag inject the ssm chaos on ec2 instance
func AWSSSMChaosByTag(clients clients.ClientSets) {

	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}

	//Fetching all the ENV passed from the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", os.Getenv("EXPERIMENT_NAME"))
	experimentEnv.GetENV(&experimentsDetails, "aws-ssm-chaos-by-tag")

	// Initialize the chaos attributes
	types.InitialiseChaosVariables(&chaosDetails)

	// Initialize Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	if experimentsDetails.EngineName != "" {
		// Get values from chaosengine. Bail out upon error, as we haven't entered exp business logic yet
		if err := types.GetValuesFromChaosEngine(&chaosDetails, clients, &resultDetails); err != nil {
			log.Errorf("Unable to initialize the probes: %v", err)
			return
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	if err := result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT"); err != nil {
		log.Errorf("Unable to create the chaosresult: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
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

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go common.AbortWatcherWithoutExit(experimentsDetails.ExperimentName, clients, &resultDetails, &chaosDetails, &eventsDetails)

	//DISPLAY THE INSTANCE INFORMATION
	log.InfoWithValues("The instance information is as follows", logrus.Fields{
		"Total Chaos Duration": experimentsDetails.ChaosDuration,
		"Chaos Namespace":      experimentsDetails.ChaosNamespace,
		"EC2 Instance Tag":     experimentsDetails.EC2InstanceTag,
		"Sequence":             experimentsDetails.Sequence,
	})

	//Verify that the instance should have permission to perform ssm api calls
	if err := ssm.CheckInstanceInformation(&experimentsDetails); err != nil {
		log.Errorf("Target instance status check failed: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		return
	}

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the pre-chaos check
		if len(resultDetails.ProbeDetails) != 0 {

			if err := probe.RunProbes(&chaosDetails, clients, &resultDetails, "PreChaos", &eventsDetails); err != nil {
				log.Errorf("Probe Failed: %v", err)
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Warning", &chaosDetails)
				if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
					log.Errorf("Failed to create %v event inside chaosengine", types.PreChaosCheck)
				}
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
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

	chaosDetails.Phase = types.ChaosInjectPhase

	if err := litmusLIB.PrepareAWSSSMChaosByTag(&experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails); err != nil {
		log.Errorf("Chaos injection failed: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		//Delete the ssm document on the given aws service monitoring docs
		if experimentsDetails.IsDocsUploaded {
			log.Info("[Recovery]: Delete the uploaded aws ssm docs")
			if err := ssm.SSMDeleteDocument(experimentsDetails.DocumentName, experimentsDetails.Region); err != nil {
				log.Errorf("Failed to delete ssm document: %v", err)
			}
		}
		return
	}

	log.Infof("[Confirmation]: %v chaos has been injected successfully", experimentsDetails.ExperimentName)
	resultDetails.Verdict = v1alpha1.ResultVerdictPassed

	chaosDetails.Phase = types.PostChaosPhase

	if chaosDetails.DefaultHealthCheck {
		//Verify the aws ec2 instance is running (post chaos)
		if err := ec2.InstanceStatusCheck(experimentsDetails.TargetInstanceIDList, experimentsDetails.Region); err != nil {
			log.Errorf("Failed to get the ec2 instance status: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			return
		}
		log.Info("[Status]: EC2 instance is in running state (post chaos)")
	}

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			if err := probe.RunProbes(&chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails); err != nil {
				log.Errorf("Probes Failed: %v", err)
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Warning", &chaosDetails)
				if eventErr := events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine"); eventErr != nil {
					log.Errorf("Failed to create %v event inside chaosengine", types.PostChaosCheck)
				}
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
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
	if err := result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT"); err != nil {
		log.Errorf("Unable to update the chaosresult:  %v", err)
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
