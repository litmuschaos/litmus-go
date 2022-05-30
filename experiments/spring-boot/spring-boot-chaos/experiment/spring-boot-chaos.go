package experiment

import (
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/spring-boot-chaos/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/spring-boot/spring-boot-chaos/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/spring-boot/spring-boot-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
	"os"
)

// Experiment contains steps to inject chaos
func Experiment(clients clients.ClientSets) {

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
		if err := probe.InitializeProbesInChaosResultDetails(&chaosDetails, clients, &resultDetails); err != nil {
			log.Errorf("Unable to initialize the probes, err: %v", err)
			return
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	if err := result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT"); err != nil {
		log.Errorf("Unable to Create the Chaos Result, err: %v", err)
		failStep := "[pre-chaos]: Failed to update the chaos result of pod-delete experiment (SOT), err: " + err.Error()
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	// Set the chaos result uid
	_ = result.SetResultUID(&resultDetails, clients, &chaosDetails)

	// generating the event in chaosResult to mark the verdict as awaited
	msg := "experiment: " + experimentsDetails.ExperimentName + ", Result: Awaited"
	types.SetResultEventAttributes(&eventsDetails, types.AwaitedVerdict, msg, "Normal", &resultDetails)
	_ = events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("[Info]: The application information is as follows", logrus.Fields{
		"Namespace":      experimentsDetails.AppNS,
		"Label":          experimentsDetails.AppLabel,
		"Chaos Duration": experimentsDetails.ChaosDuration,
	})

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal and generate the required events and result
	go common.AbortWatcher(experimentsDetails.ExperimentName, clients, &resultDetails, &chaosDetails, &eventsDetails)


	// Select targeted pods
	log.Infof("[PreCheck]: Geting targeted pods list")
	if err := litmusLIB.SetTargetPodList(&experimentsDetails, clients, &chaosDetails); err != nil {
		log.Errorf("Failed to get target pod list, err: %v", err)
		failStep := "[pre-chaos]: Failed to get pod list, err: " + err.Error()
 		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, "Pods: Not Found", "Warning", &chaosDetails)
 		_ = events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
 		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
 		return
	}
	podNames := make([]string, 0, 1)
	for _, pod := range experimentsDetails.TargetPodList.Items {
		podNames = append(podNames, pod.Name)
	}
	log.Infof("[PreCheck]: Target pods list for chaos, %v", podNames)

	// Check if the targeted pods have the chaos monkey endpoint
	log.Infof("[PreCheck]: Checking for ChaosMonkey endpoint in target pods")
	if _, err := litmusLIB.CheckChaosMonkey(experimentsDetails.ChaosMonkeyPort, experimentsDetails.ChaosMonkeyPath, experimentsDetails.TargetPodList); err != nil {
		log.Errorf("Some target pods don't have the chaos monkey endpoint, err: %v", err)
	}

	//PRE-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	if err := status.AUTStatusCheck(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.TargetContainer, experimentsDetails.Timeout, experimentsDetails.Delay, clients, &chaosDetails); err != nil {
		log.Errorf("Application status check failed, err: %v", err)
		failStep := "[pre-chaos]: Failed to verify that the AUT (Application Under Test) is in running state, err: " + err.Error()
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, "AUT: Not Running", "Warning", &chaosDetails)
		_ = events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the pre-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			if err := probe.RunProbes(&chaosDetails, clients, &resultDetails, "PreChaos", &eventsDetails); err != nil {
				log.Errorf("Probe Failed, err: %v", err)
				failStep := "[pre-chaos]: Failed while running probes, err: " + err.Error()
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Warning", &chaosDetails)
				_ = events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
				result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}
		// generating the events for the pre-chaos check
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Normal", &chaosDetails)
		_ = events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}


	// Including the litmus lib
	switch experimentsDetails.ChaosLib {
	case "litmus":
		if err := litmusLIB.PrepareChaos(&experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails); err != nil {
			log.Errorf("Chaos injection failed, err: %v", err)
			failStep := "[chaos]: Failed inside the chaoslib, err: " + err.Error()
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
	default:
		log.Error("[Invalid]: Please Provide the correct LIB")
		failStep := "[chaos]: no match found for specified lib"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}


	// POST-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	if err := status.AUTStatusCheck(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.TargetContainer, experimentsDetails.Timeout, experimentsDetails.Delay, clients, &chaosDetails); err != nil {
		log.Errorf("Application status check failed, err: %v", err)
		failStep := "[post-chaos]: Failed to verify that the AUT (Application Under Test) is running, err: " + err.Error()
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, "AUT: Not Running", "Warning", &chaosDetails)
		_ = events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	if experimentsDetails.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			if err := probe.RunProbes(&chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails); err != nil {
				log.Errorf("Probes Failed, err: %v", err)
				failStep := "[post-chaos]: Failed while running probes, err: " + err.Error()
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Warning", &chaosDetails)
				_ = events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
				result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}

		// generating post chaos event
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Normal", &chaosDetails)
		_ = events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	//Updating the chaosResult in the end of experiment
	log.Infof("[The End]: Updating the chaos result of %v experiment (EOT)", experimentsDetails.ExperimentName)
	if err := result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT"); err != nil {
		log.Errorf("Unable to Update the Chaos Result, err: %v", err)
		return
	}

	// generating the event in chaosResult to mark the verdict as pass/fail
	msg = "experiment: " + experimentsDetails.ExperimentName + ", Result: " + string(resultDetails.Verdict)
	reason := types.PassVerdict
	eventType := "Normal"
	if resultDetails.Verdict != "Pass" {
		reason = types.FailVerdict
		eventType = "Warning"
	}
	types.SetResultEventAttributes(&eventsDetails, reason, msg, eventType, &resultDetails)
	_ = events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	if experimentsDetails.EngineName != "" {
		msg := experimentsDetails.ExperimentName + " experiment has been " + string(resultDetails.Verdict) + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		_ = events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}
}
