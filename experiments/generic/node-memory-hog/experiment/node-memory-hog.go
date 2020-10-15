package experiment

import (
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/node-memory-hog/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/generic/node-memory-hog/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-memory-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
	"k8s.io/klog"
)

// NodeMemoryHog inject the node-memory-hog chaos
func NodeMemoryHog(clients clients.ClientSets) {

	var err error
	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}

	//Fetching all the ENV passed from the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", experimentsDetails.ExperimentName)
	experimentEnv.GetENV(&experimentsDetails)

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Intialise the probe details. Bail out upon error, as we haven't entered exp business logic yet
	if err = probe.InitializeProbesInChaosResultDetails(&chaosDetails, clients, &resultDetails); err != nil {
		log.Fatalf("Unable to initialize the probes, err: %v", err)
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result, err: %v", err)
		failStep := "Updating the chaos result of node-memory-hog experiment (SOT)"
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
		"Namespace":         experimentsDetails.AppNS,
		"Label":             experimentsDetails.AppLabel,
		"Chaos Duration":    experimentsDetails.ChaosDuration,
		"Ramp Time":         experimentsDetails.RampTime,
		"Memory Percentage": experimentsDetails.MemoryPercentage,
	})

	// Calling AbortWatcher go routine, it will continuously watch for the abort signal for the entire chaos duration and generate the required events and result
	// It is being invoked here, as opposed to within the chaoslib, as these experiments do not need additional recovery/chaos revert steps like in case of network experiments
	go common.AbortWatcher(experimentsDetails.ExperimentName, clients, &resultDetails, &chaosDetails, &eventsDetails)

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
				log.Errorf("Probe failed, err: %v", err)
				failStep := "Failed while adding probe"
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

	// Including the litmus lib for node-memory-hog
	if experimentsDetails.ChaosLib == "litmus" {
		err = litmusLIB.PrepareNodeMemoryHog(&experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails)
		if err != nil {
			log.Errorf("[Error]: node memory hog failed, err: %v", err)
			failStep := "failed in chaos injection phase"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
		log.Info("[Confirmation]: Memory of the application node has been stressed successfully")
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
		klog.V(0).Infof("Application status check failed, err: %v", err)
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
				log.Errorf("Unable to Add the probes, err: %v", err)
				failStep := "Failed while adding probe"
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

}
