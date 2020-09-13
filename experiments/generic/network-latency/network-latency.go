package main

import (
	"github.com/litmuschaos/litmus-go/chaoslib/litmus/network_latency"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	environment "github.com/litmuschaos/litmus-go/pkg/generic/network-latency/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-latency/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		// ForceColors:            true,
		DisableSorting:         true,
		DisableLevelTruncation: true,
	})
}

func main() {

	var err error
	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}
	clients := clients.ClientSets{}

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Fatalf("Unable to Get the kubeconfig due to %v", err)
	}

	//Fetching all the ENV passed for the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", experimentsDetails.ExperimentName)
	environment.GetENV(&experimentsDetails, "network-service-latency")

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, experimentsDetails.EngineName, experimentsDetails.ExperimentName)

	// //Updating the chaos result in the beggining of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result due to %v", err)
		failStep := "Updating the chaos result ofnetwork-service-latency experiment (SOT)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("The application informations are as follows", logrus.Fields{
		"Namespace":      experimentsDetails.AppNS,
		"Label":          experimentsDetails.AppLabel,
		"Chaos Duration": experimentsDetails.ChaosDuration,
		"Ramp Time":      experimentsDetails.RampTime,
	})

	// ADD A PRE-CHAOS CHECK OF YOUR CHOICE HERE
	// POD STATUS CHECKS FOR THE APPLICATION UNDER TEST AND AUXILIARY APPLICATIONS ARE ADDED BY DEFAULT

	//PRE-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		log.Errorf("Application status check failed due to %v\n", err)
		failStep := "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	if experimentsDetails.EngineName != "" {
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, "AUT is Running successfully", "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// INVOKING THE CHAOS NETWORK HOG EXPERIMENT

	// Including the litmus lib for pod-delete
	if experimentsDetails.ChaosLib == "litmus" {
		err = network_latency.PrepareNetwork(&experimentsDetails, clients, &resultDetails)
		if err != nil {
			log.Errorf("Chaos injection failed due to %v\n", err)
			failStep := "Including the litmus lib for network latency"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		} else {
			log.Info("[Confirmation]: Network experiment has been concluded successfully")
			resultDetails.Verdict = "Pass"
		}
	} else {
		log.Error("[Invalid]: Please Provide the correct LIB")
		failStep := "Including the litmus lib for network latency"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		log.Errorf("Application status check failed due to %v\n", err)
		failStep := "Verify that the AUT (Application Under Test) is running (post-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	if experimentsDetails.EngineName != "" {
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, "AUT is Running successfully", "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	//Updating the chaosResult in the end of experiment
	log.Infof("[The End]: Updating the chaos result of %v experiment (EOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
	if err != nil {
		log.Fatalf("Unable to Update the Chaos Result due to %v\n", err)
	}
	if experimentsDetails.EngineName != "" {
		msg := experimentsDetails.ExperimentName + " experiment has been " + resultDetails.Verdict + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	msg := experimentsDetails.ExperimentName + " experiment has been " + resultDetails.Verdict + "ed"
	types.SetResultEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")
}
