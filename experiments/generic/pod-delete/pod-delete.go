package main

import (
	 "github.com/litmuschaos/litmus-go/chaoslib/litmus/pod_delete"
	 "github.com/litmuschaos/litmus-go/pkg/status"
	 "github.com/litmuschaos/litmus-go/pkg/result"
	 "github.com/litmuschaos/litmus-go/pkg/environment"
	 "github.com/litmuschaos/litmus-go/pkg/types"
	 "github.com/litmuschaos/litmus-go/pkg/events"
	 log "github.com/sirupsen/logrus"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.TextFormatter{
		// DisableColors: true,
		FullTimestamp: true,
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
		log.WithFields(log.Fields{}).Fatal("Unable to Get the kubeconfig due to %v", err)
	}

	//Fetching all the ENV passed for the runner pod
	log.WithFields(log.Fields{}).Infof("[PreReq]: Getting the ENV for the %v experiment",experimentsDetails.ExperimentName)
	environment.GetENV(&experimentsDetails, "pod-delete")

	recorder, err := events.NewEventRecorder(clients, experimentsDetails)
	if err != nil {
		log.WithFields(log.Fields{}).Warn("Unable to initiate EventRecorder for chaos-experiment, would not be able to add events")
	}

	// Intialise Chaos Result Parameters
	environment.SetResultAttributes(&resultDetails,&experimentsDetails)

	//Updating the chaos result in the beggining of experiment
	log.WithFields(log.Fields{}).Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)",experimentsDetails.ExperimentName)
	err = result.ChaosResult(&experimentsDetails, clients,&resultDetails,"SOT")
	if err != nil {
		log.WithFields(log.Fields{}).Errorf("Unable to Create the Chaos Result due to %v", err)
		resultDetails.FailStep = "Updating the chaos result of pod-delete experiment (SOT)"
		err = result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT"); return
	}

	//DISPLAY THE APP INFORMATION
	log.WithFields(log.Fields{
		"Namespace": experimentsDetails.AppNS,
		"Label": experimentsDetails.AppLabel,
		"Ramp Time": experimentsDetails.RampTime, 
	  }).Info("The application informations are as follows:")
	 
	//PRE-CHAOS APPLICATION STATUS CHECK
	log.WithFields(log.Fields{}).Info("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		log.WithFields(log.Fields{}).Errorf("Application status check failed due to %v\n", err)
		resultDetails.FailStep = "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT"); return
	}

	if experimentsDetails.EngineName != ""{
	recorder.PreChaosCheck(experimentsDetails)
}

	// Including the litmus lib for pod-delete
	if experimentsDetails.ChaosLib == "litmus" {
		err = pod_delete.PreparePodDelete(&experimentsDetails, clients,&resultDetails,recorder)
		if err != nil {
			log.WithFields(log.Fields{}).Errorf("Chaos injection failed due to %v\n", err)
			result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT"); return
		}
		log.WithFields(log.Fields{}).Info("[Confirmation]: The application pod has been deleted successfully")
		resultDetails.Verdict="Pass"
	} else {
		log.WithFields(log.Fields{}).Errorf("[Invalid]: Please Provide the correct LIB")
		resultDetails.FailStep = "Including the litmus lib for pod-delete"
		result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT"); return
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	log.WithFields(log.Fields{}).Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		log.WithFields(log.Fields{}).Errorf("Application status check failed due to %v\n", err)
		resultDetails.FailStep = "Verify that the AUT (Application Under Test) is running (post-chaos)"
		result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT"); return
	}
	if experimentsDetails.EngineName != ""{
	recorder.PostChaosCheck(experimentsDetails)
	}

	//Updating the chaosResult in the end of experiment
	log.WithFields(log.Fields{}).Infof("[The End]: Updating the chaos result of %v experiment (EOT)",experimentsDetails.ExperimentName)
	err = result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT")
	if err != nil {
		log.WithFields(log.Fields{}).Fatal("Unable to Update the Chaos Result due to %v\n", err)
	}
	if experimentsDetails.EngineName != ""{
	recorder.Summary(&experimentsDetails,&resultDetails)
	}
}
