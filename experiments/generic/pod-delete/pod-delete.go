package main

import (
	"github.com/litmuschaos/litmus-go/chaoslib/litmus/pod_delete"
	"github.com/litmuschaos/litmus-go/pkg/environment"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	experimentenv "github.com/litmuschaos/litmus-go/pkg/pod-delete/environment"
	experimenttypes "github.com/litmuschaos/litmus-go/pkg/pod-delete/types"
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
	experimentsDetails := experimenttypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	clients := environment.ClientSets{}

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Fatalf("Unable to Get the kubeconfig due to %v", err)
	}

	//Fetching all the ENV passed for the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", experimentsDetails.ExperimentName)
	experimentenv.GetENV(&experimentsDetails, "pod-delete")

	// Intialise Chaos Result Parameters
	environment.SetResultAttributes(&resultDetails, experimentsDetails.EngineName, eventsDetails.ExperimentName)

	// Intialise events Parameters
	experimentenv.InitialiseEventAttributes(&eventsDetails, &experimentsDetails)

	//Updating the chaos result in the beggining of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&eventsDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result due to %v", err)
		resultDetails.FailStep = "Updating the chaos result of pod-delete experiment (SOT)"
		err = result.ChaosResult(&eventsDetails, clients, &resultDetails, "EOT")
		return
	}

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("The application informations are as follows", logrus.Fields{
		"Namespace": experimentsDetails.AppNS,
		"Label":     experimentsDetails.AppLabel,
		"Ramp Time": experimentsDetails.RampTime,
	})

	//PRE-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		log.Errorf("Application status check failed due to %v\n", err)
		resultDetails.FailStep = "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		result.ChaosResult(&eventsDetails, clients, &resultDetails, "EOT")
		return
	}
	if experimentsDetails.EngineName != "" {
		environment.SetEventAttributes(&eventsDetails, types.PreChaosCheck, "AUT is Running successfully")
		events.GenerateEvents(&eventsDetails, clients)
	}

	// Including the litmus lib for pod-delete
	if experimentsDetails.ChaosLib == "litmus" {
		err = pod_delete.PreparePodDelete(&experimentsDetails, clients, &resultDetails, &eventsDetails)
		if err != nil {
			log.Errorf("Chaos injection failed due to %v\n", err)
			resultDetails.FailStep = "Including the litmus lib for pod-delete"
			result.ChaosResult(&eventsDetails, clients, &resultDetails, "EOT")
			return
		}
		log.Info("[Confirmation]: The application pod has been deleted successfully")
		resultDetails.Verdict = "Pass"
	} else {
		log.Error("[Invalid]: Please Provide the correct LIB")
		resultDetails.FailStep = "Including the litmus lib for pod-delete"
		result.ChaosResult(&eventsDetails, clients, &resultDetails, "EOT")
		return
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		log.Errorf("Application status check failed due to %v\n", err)
		resultDetails.FailStep = "Verify that the AUT (Application Under Test) is running (post-chaos)"
		result.ChaosResult(&eventsDetails, clients, &resultDetails, "EOT")
		return
	}
	if experimentsDetails.EngineName != "" {
		environment.SetEventAttributes(&eventsDetails, types.PostChaosCheck, "AUT is Running successfully")
		events.GenerateEvents(&eventsDetails, clients)
	}

	//Updating the chaosResult in the end of experiment
	log.Infof("[The End]: Updating the chaos result of %v experiment (EOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&eventsDetails, clients, &resultDetails, "EOT")
	if err != nil {
		log.Fatalf("Unable to Update the Chaos Result due to %v\n", err)
	}
	if experimentsDetails.EngineName != "" {
		msg := experimentsDetails.ExperimentName + "experiment has been" + resultDetails.Verdict + "ed"
		environment.SetEventAttributes(&eventsDetails, types.Summary, msg)
		events.GenerateEvents(&eventsDetails, clients)
	}
}
