package main

import (
	"k8s.io/klog"

	litmus "github.com/litmuschaos/litmus-go/chaoslib/litmus"
	utils "github.com/litmuschaos/litmus-go/pkg/utils"
)

func main() {
	var err error
	//flag contains the verdict of the experiment
	flag := "Fail"
	failStep:= "N/A"
	experimentsDetails := utils.ExperimentDetails{}
	clients := utils.ClientSets{}

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		klog.V(0).Infof("Unable to Get the kubeconfig due to %v\n", err)
	}

	//Fetching all the ENV passed for the runner pod
	klog.V(0).Infof("[PreReq]: Getting the ENV for the %v experiment",experimentsDetails.ExperimentName)
	utils.GetENV(&experimentsDetails, "pod-delete")

	//Updating the chaos result in the beggining of experiment
	klog.V(0).Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)",experimentsDetails.ExperimentName)
	err = utils.CreateChaosResultStart(&experimentsDetails, clients)
	if err != nil {
		klog.V(0).Infof("Unable to Create the Chaos Result due to %v\n", err)
		failStep = "Updating the chaos result of pod-delete experiment (SOT)"
		utils.UpdateChaosResultMiddle(&experimentsDetails, clients, flag,failStep); return
	}

	//DISPLAY THE APP INFORMATION
	klog.V(0).Infof("[Info]: Display the application information passed via the test job")
	klog.V(0).Infof("The application info is as follows: Namespace: %v, Label: %v, Ramp Time: %v", experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.RampTime)

	//PRE-CHAOS APPLICATION STATUS CHECK
	klog.V(0).Infof("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	err = utils.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		klog.V(0).Infof("Application status check failed due to %v\n", err)
		failStep = "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		utils.UpdateChaosResultMiddle(&experimentsDetails, clients, flag,failStep); return
	}

	//PRE-CHAOS AUXILIARY APPLICATION LIVENESS CHECK
	if experimentsDetails.AuxiliaryAppInfo == "no" {
		klog.V(0).Infof("[Status]: Verify that the Auxiliary Applications are running (pre-chaos)")
		err = utils.CheckAuxiliaryApplicationStatus(&experimentsDetails, clients)
		if err != nil {
			klog.V(0).Infof("Auxiliary application status check failed due to %v\n", err)
			failStep = "Verify that the Auxiliary Applications are running (pre-chaos)"
			utils.UpdateChaosResultMiddle(&experimentsDetails, clients, flag,failStep); return
		}
	}

	//Including the litmus lib for pod-delete
	if experimentsDetails.ChaosLib == "litmus" {
		err = litmus.PreparePodDelete(&experimentsDetails, clients,&failStep)
		if err != nil {
			klog.V(0).Infof("Chaos injection failed due to %v\n", err)
			utils.UpdateChaosResultMiddle(&experimentsDetails, clients, flag,failStep); return
		}
		klog.V(0).Infof("[Confirmation]: The application pod has been deleted successfully")
		flag = "Pass"
	} else {
		klog.V(0).Infof("[Invalid]: Please Provide the correct LIB")
		failStep = "Including the litmus lib for pod-delete"
		utils.UpdateChaosResultMiddle(&experimentsDetails, clients, flag,failStep); return
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	klog.V(0).Infof("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	err = utils.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
	if err != nil {
		klog.V(0).Infof("Application status check failed due to %v\n", err)
		failStep = "Verify that the AUT (Application Under Test) is running (post-chaos)"
		utils.UpdateChaosResultMiddle(&experimentsDetails, clients,"Fail",failStep); return
	}

	//Post-CHAOS AUXILIARY APPLICATION STATUS CHECK
	if experimentsDetails.AuxiliaryAppInfo == "no" {
		klog.V(0).Infof("[Status]: Verify that the Auxiliary Applications are running (post-chaos)")
		err = utils.CheckAuxiliaryApplicationStatus(&experimentsDetails, clients)
		if err != nil {
			klog.V(0).Infof("Auxiliary application status check failed due to %v\n", err)
			failStep = "Verify that the Auxiliary Applications are running (post-chaos)"
			utils.UpdateChaosResultMiddle(&experimentsDetails, clients,"Fail",failStep); return
		}
	}

	//Updating the chaosResult in the end of experiment
	klog.V(0).Infof("[The End]: Updating the chaos result of %v experiment (EOT)",experimentsDetails.ExperimentName)
	err = utils.UpdateChaosResultEnd(&experimentsDetails, clients, flag,failStep)
	if err != nil {
		klog.V(0).Infof("Unable to Update the Chaos Result due to %v\n", err)
		return
	}

}
