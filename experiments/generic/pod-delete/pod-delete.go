package main

import (
	"k8s.io/klog"

	 "github.com/litmuschaos/litmus-go/chaoslib/litmus/pod_delete"
	 "github.com/litmuschaos/litmus-go/pkg/status"
	 "github.com/litmuschaos/litmus-go/pkg/result"
	 "github.com/litmuschaos/litmus-go/pkg/environment"
	 "github.com/litmuschaos/litmus-go/pkg/types"
	 "github.com/litmuschaos/litmus-go/pkg/events"
)

func main() {

	var err error
	experimentsDetails := types.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	clients := environment.ClientSets{}
	
	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		klog.V(0).Infof("Unable to Get the kubeconfig due to %v\n", err)
	}

	//Fetching all the ENV passed for the runner pod
	klog.V(0).Infof("[PreReq]: Getting the ENV for the %v experiment",experimentsDetails.ExperimentName)
	environment.GetENV(&experimentsDetails, "pod-delete")

	recorder, err := events.NewEventRecorder(clients, experimentsDetails)
	if err != nil {
		klog.Errorf("Unable to initiate EventRecorder for chaos-experiment, would not be able to add events")
	}

	// Intialise Chaos Result Parameters
	environment.SetResultAttributes(&resultDetails,&experimentsDetails)

	//Updating the chaos result in the beggining of experiment
	klog.V(0).Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)",experimentsDetails.ExperimentName)
	err = result.ChaosResult(&experimentsDetails, clients,&resultDetails,"SOT")
	if err != nil {
		klog.V(0).Infof("Unable to Create the Chaos Result due to %v\n", err)
		resultDetails.FailStep = "Updating the chaos result of pod-delete experiment (SOT)"
		err = result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT"); return
	}

	//DISPLAY THE APP INFORMATION
	klog.V(0).Infof("[Info]: Display the application information passed via the test job")
	klog.V(0).Infof("The application info is as follows: Namespace: %v, Label: %v, Ramp Time: %v", experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.RampTime)

	//PRE-CHAOS APPLICATION STATUS CHECK
	klog.V(0).Infof("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		klog.V(0).Infof("Application status check failed due to %v\n", err)
		resultDetails.FailStep = "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT"); return
	}
	if experimentsDetails.EngineName != "" {
	recorder.PreChaosCheck(experimentsDetails)
	}

	// Including the litmus lib for pod-delete
	if experimentsDetails.ChaosLib == "litmus" {
		err = pod_delete.PreparePodDelete(&experimentsDetails, clients,&resultDetails,recorder)
		if err != nil {
			klog.V(0).Infof("Chaos injection failed due to %v\n", err)
			result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT"); return
		}
		klog.V(0).Infof("[Confirmation]: The application pod has been deleted successfully")
		resultDetails.Verdict="Pass"
	} else {
		klog.V(0).Infof("[Invalid]: Please Provide the correct LIB")
		resultDetails.FailStep = "Including the litmus lib for pod-delete"
		result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT"); return
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	klog.V(0).Infof("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		klog.V(0).Infof("Application status check failed due to %v\n", err)
		resultDetails.FailStep = "Verify that the AUT (Application Under Test) is running (post-chaos)"
		result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT"); return
	}
	if experimentsDetails.EngineName != "" {
	recorder.PostChaosCheck(experimentsDetails)
	}

	//Updating the chaosResult in the end of experiment
	klog.V(0).Infof("[The End]: Updating the chaos result of %v experiment (EOT)",experimentsDetails.ExperimentName)
	err = result.ChaosResult(&experimentsDetails, clients,&resultDetails,"EOT")
	if err != nil {
		klog.V(0).Infof("Unable to Update the Chaos Result due to %v\n", err)
		return
	}
	if experimentsDetails.EngineName != "" {
	recorder.Summary(&experimentsDetails,&resultDetails)
	}
}
