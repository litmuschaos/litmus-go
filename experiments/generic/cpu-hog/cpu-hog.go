package main

import (
	"github.com/litmuschaos/litmus-go/chaoslib/litmus/cpu_hog"
	"github.com/litmuschaos/litmus-go/pkg/environment"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
	"k8s.io/klog"
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
	experimentsDetails := types.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	clients := environment.ClientSets{}

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Fatalf("Unable to Get the kubeconfig due to %v", err)
	}

	//Fetching all the ENV passed for the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", experimentsDetails.ExperimentName)
	environment.GetENV(&experimentsDetails, "cpu-hog")

	recorder, err := events.NewEventRecorder(clients, experimentsDetails)
	if err != nil {
		log.Warn("Unable to initiate EventRecorder for chaos-experiment, would not be able to add events")
	}

	// Intialise Chaos Result Parameters
	environment.SetResultAttributes(&resultDetails, &experimentsDetails)

	//Updating the chaos result in the beggining of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&experimentsDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result due to %v", err)
		resultDetails.FailStep = "Updating the chaos result of cpu-hog experiment (SOT)"
		err = result.ChaosResult(&experimentsDetails, clients, &resultDetails, "EOT")
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
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		log.Errorf("Application status check failed due to %v\n", err)
		resultDetails.FailStep = "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		result.ChaosResult(&experimentsDetails, clients, &resultDetails, "EOT")
		return
	}

	if experimentsDetails.EngineName != "" {
		recorder.PreChaosCheck(experimentsDetails)
	}

	// Including the litmus lib for cpu-hog
	if experimentsDetails.ChaosLib == "litmus" {
		err = cpu_hog.PrepareCPUstress(&experimentsDetails, clients, &resultDetails, recorder)
		if err != nil {
			log.Errorf("[Error]: CPU hog failed due to %v\n", err)
			resultDetails.FailStep = "CPU hog Chaos injection failed"
			resultDetails.Verdict = "Fail"
			result.ChaosResult(&experimentsDetails, clients, &resultDetails, "EOT")
			return
		} else {
			log.Info("[Confirmation]: CPU of the application pod has been stressed successfully")
			resultDetails.Verdict = "Pass"
		}
	} else {
		log.Error("[Invalid]: Please Provide the correct LIB")
		resultDetails.FailStep = "Including the litmus lib for cpu-hog"
		result.ChaosResult(&experimentsDetails, clients, &resultDetails, "EOT")
		return
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		klog.V(0).Infof("Application status check failed due to %v\n", err)
		resultDetails.FailStep = "Verify that the AUT (Application Under Test) is running (post-chaos)"
		result.ChaosResult(&experimentsDetails, clients, &resultDetails, "EOT")
		return
	}

	if experimentsDetails.EngineName != "" {
		recorder.PostChaosCheck(experimentsDetails)
	}

	//Updating the chaosResult in the end of experiment
	log.Infof("[The End]: Updating the chaos result of %v experiment (EOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&experimentsDetails, clients, &resultDetails, "EOT")
	if err != nil {
		log.Fatalf("Unable to Update the Chaos Result due to %v\n", err)
	}
	if experimentsDetails.EngineName != "" {
		recorder.Summary(&experimentsDetails, &resultDetails)
	}
}
