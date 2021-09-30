package experiment

import (
	"os"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/azure-instance-stop/lib"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/azure/instance-stop/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/instance-stop/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	azureCommon "github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"
	azureStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure/instance"

	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
)

// AzureInstanceStop inject the azure instance stop chaos
func AzureInstanceStop(clients clients.ClientSets) {

	var err error
	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}

	//Fetching all the ENV passed from the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", os.Getenv("EXPERIMENT_NAME"))
	experimentEnv.GetENV(&experimentsDetails)

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	if experimentsDetails.EngineName != "" {
		// Intialise the probe details. Bail out upon error, as we haven't entered exp business logic yet
		if err = probe.InitializeProbesInChaosResultDetails(&chaosDetails, clients, &resultDetails); err != nil {
			log.Errorf("Unable to initialize the probes, err: %v", err)
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result, err: %v", err)
		failStep := "Updating the chaos result of azure instance stop experiment (SOT)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go common.AbortWatcherWithoutExit(experimentsDetails.ExperimentName, clients, &resultDetails, &chaosDetails, &eventsDetails)

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("The application information is as follows", logrus.Fields{
		"Chaos Duration": experimentsDetails.ChaosDuration,
		"Resource Group": experimentsDetails.ResourceGroup,
		"Instance Name":  experimentsDetails.AzureInstanceName,
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

	//PRE-CHAOS AUXILIARY APPLICATION STATUS CHECK
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running (pre-chaos)")
		err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			log.Errorf("Auxiliary Application status check failed, err: %v", err)
			failStep := "Verify that the Auxiliary Applications are running (pre-chaos)"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
	}

	// Setting up Azure Subscription ID
	if experimentsDetails.SubscriptionID, err = azureCommon.GetSubscriptionID(); err != nil {
		log.Errorf("fail to get the subscription id, err: %v", err)
		failStep := "Getting the subscription ID for authentication"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	// generating the event in chaosresult to marked the verdict as awaited
	msg := "experiment: " + experimentsDetails.ExperimentName + ", Result: Awaited"
	types.SetResultEventAttributes(&eventsDetails, types.AwaitedVerdict, msg, "Normal", &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

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

	//Verify the azure target instance is running (pre-chaos)
	if err := azureStatus.InstanceStatusCheckByName(&experimentsDetails); err != nil {
		log.Errorf("failed to get the azure instance status, err: %v", err)
		failStep := "Verify the azure instance status (pre-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}
	log.Info("[Status]: Azure instance(s) is in running state (pre-chaos)")

	// Including the litmus lib for azure instance stopping
	switch experimentsDetails.ChaosLib {
	case "litmus":
		if err = litmusLIB.PrepareAzureStop(&experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails); err != nil {
			log.Errorf("Chaos injection failed, err: %v", err)
			failStep := "failed in chaos injection phase"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
	default:
		log.Error("[Invalid]: Please Provide the correct LIB")
		failStep := "no match found for specified lib"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	log.Info("[Confirmation]: Azure instance stop chaos has been injected successfully")
	resultDetails.Verdict = v1alpha1.ResultVerdictPassed

	//Verify the azure instance is running (post chaos)
	if err = azureStatus.InstanceStatusCheckByName(&experimentsDetails); err != nil {
		log.Errorf("failed to get the azure instance status, err: %v", err)
		failStep := "Verify the azure instance status (post-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}
	log.Info("[Status]: Azure instance is in running state (post chaos)")

	//POST-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	if err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		log.Errorf("Application status check failed, err: %v", err)
		failStep := "Verify that the AUT (Application Under Test) is running (post-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	//POST-CHAOS AUXILIARY APPLICATION STATUS CHECK
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running (post-chaos)")
		if err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			log.Errorf("Auxiliary Application status check failed, err: %v", err)
			failStep := "Verify that the Auxiliary Applications are running (post-chaos)"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
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
		log.Errorf("Unable to Update the Chaos Result, err:  %v", err)
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
