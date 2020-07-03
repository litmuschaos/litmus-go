package main

import (
	"github.com/litmuschaos/litmus-go/chaoslib/litmus/pod_cpu_hog"
	"github.com/litmuschaos/litmus-go/pkg/environment"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/pod-cpu-hog/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/pod-cpu-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
	"k8s.io/klog"
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
	chaosDetails := types.ChaosDetails{}
	clients := environment.ClientSets{}

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Fatalf("Unable to Get the kubeconfig due to %v", err)
	}

	//Fetching all the ENV passed from the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", experimentsDetails.ExperimentName)
	experimentEnv.GetENV(&experimentsDetails, "pod-cpu-hog")

	// Intialise Chaos Result Parameters
	environment.SetResultAttributes(&resultDetails, experimentsDetails.EngineName, experimentsDetails.ExperimentName)

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	//Updating the chaos result in the beggining of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result due to %v", err)
		resultDetails.FailStep = "Updating the chaos result of pod-cpu-hog experiment (SOT)"
		err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("The application informations are as follows", logrus.Fields{
		"Namespace":      experimentsDetails.AppNS,
		"Label":          experimentsDetails.AppLabel,
		"Chaos Duration": experimentsDetails.ChaosDuration,
		"Ramp Time":      experimentsDetails.RampTime,
	})

	//PRE-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		log.Errorf("Application status check failed due to %v\n", err)
		resultDetails.FailStep = "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	if experimentsDetails.EngineName != "" {
		environment.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, "AUT is Running successfully", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// Including the litmus lib for pod-cpu-hog
	if experimentsDetails.ChaosLib == "litmus" {
		err = pod_cpu_hog.PrepareCPUstress(&experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails)
		if err != nil {
			log.Errorf("[Error]: CPU hog failed due to %v\n", err)
			resultDetails.FailStep = "CPU hog Chaos injection failed"
			resultDetails.Verdict = "Fail"
			result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
			return
		}
		log.Info("[Confirmation]: CPU of the application pod has been stressed successfully")
		resultDetails.Verdict = "Pass"
	} else {
		log.Error("[Invalid]: Please Provide the correct LIB")
		resultDetails.FailStep = "Including the litmus lib for pod-cpu-hog"
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		klog.V(0).Infof("Application status check failed due to %v\n", err)
		resultDetails.FailStep = "Verify that the AUT (Application Under Test) is running (post-chaos)"
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	if experimentsDetails.EngineName != "" {
		environment.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, "AUT is Running successfully", &chaosDetails)
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
		environment.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	msg := experimentsDetails.ExperimentName + " experiment has been " + resultDetails.Verdict + "ed"
	environment.SetResultEventAttributes(&eventsDetails, types.Summary, msg, &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")
}
