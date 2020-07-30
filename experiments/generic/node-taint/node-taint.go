package main

import (
	node_taint "github.com/litmuschaos/litmus-go/chaoslib/litmus/node_taint"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/generic/node-taint/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-taint/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:          true,
		DisableSorting:         true,
		DisableLevelTruncation: true,
	})
}

func main() {

	var err error
	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	clients := clients.ClientSets{}
	chaosDetails := types.ChaosDetails{}

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Fatalf("Unable to Get the kubeconfig due to %v", err)
	}

	//Fetching all the ENV passed from the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", experimentsDetails.ExperimentName)
	experimentEnv.GetENV(&experimentsDetails, "node-taint")

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result due to %v", err)
		failStep := "Updating the chaos result of node-taint experiment (SOT)"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("The application informations are as follows", logrus.Fields{
		"Namespace": experimentsDetails.AppNS,
		"Label":     experimentsDetails.AppLabel,
		"Ramp Time": experimentsDetails.RampTime,
	})

	//PRE-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		log.Errorf("Application status check failed due to %v\n", err)
		failStep := "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	//PRE-CHAOS AUXILIARY APPLICATION STATUS CHECK
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running (pre-chaos)")
		err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			log.Errorf("Auxiliary Application status check failed due to %v", err)
			failStep := "Verify that the Auxiliary Applications are running (pre-chaos)"
			types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
			result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
			return
		}
	}

	if experimentsDetails.EngineName != "" {
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, "AUT is Running successfully", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// Including the litmus lib for node-taint
	if experimentsDetails.ChaosLib == "litmus" {
		err = node_taint.PrepareNodeTaint(&experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails)
		if err != nil {
			log.Errorf("Chaos injection failed due to %v\n", err)
			failStep := "Including the litmus lib for node-taint"
			types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
			result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
			return
		}
		log.Info("[Confirmation]: The application node has been tainted successfully")
		resultDetails.Verdict = "Pass"
	} else {
		log.Error("[Invalid]: Please Provide the correct LIB")
		failStep := "Including the litmus lib for node-taint"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		log.Errorf("Application status check failed due to %v\n", err)
		failStep := "Verify that the AUT (Application Under Test) is running (post-chaos)"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	//POST-CHAOS AUXILIARY APPLICATION STATUS CHECK
	if experimentsDetails.AuxiliaryAppInfo != "" {
		log.Info("[Status]: Verify that the Auxiliary Applications are running (post-chaos)")
		err = status.CheckAuxiliaryApplicationStatus(experimentsDetails.AuxiliaryAppInfo, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			log.Errorf("Auxiliary Application status check failed due to %v", err)
			failStep := "Verify that the Auxiliary Applications are running (post-chaos)"
			types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
			result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
			return
		}
	}

	if experimentsDetails.EngineName != "" {
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, "AUT is Running successfully", &chaosDetails)
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
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	msg := experimentsDetails.ExperimentName + " experiment has been " + resultDetails.Verdict + "ed"
	types.SetResultEventAttributes(&eventsDetails, types.Summary, msg, &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")
}
