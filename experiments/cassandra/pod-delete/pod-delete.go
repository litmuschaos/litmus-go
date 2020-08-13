package main

import (
	"github.com/litmuschaos/litmus-go/chaoslib/litmus/pod_delete"
	"github.com/litmuschaos/litmus-go/pkg/apps/cassandra"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
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
	var ResourceVersionBefore string
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
	experimentEnv.GetENV(&experimentsDetails, "pod-delete")

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result due to %v", err)
		failStep := "Updating the chaos result of pod-delete experiment (SOT)"
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
	if experimentsDetails.EngineName != "" {
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, "AUT is Running successfully", "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// Checking the load distribution on the ring (pre-chaos)
	log.Info("[Status]: Checking the load distribution on the ring (pre-chaos)")
	err = cassandra.NodeToolStatusCheck(&experimentsDetails, clients)
	if err != nil {
		log.Fatalf("[Status]: Chaos node tool status check is failed due to %v\n", err)
	}

	// Cassandra liveness check
	if experimentsDetails.CassandraLivenessCheck == "enabled" {
		ResourceVersionBefore, err = cassandra.LivenessCheck(&experimentsDetails, clients)
		if err != nil {
			log.Fatalf("[Liveness]: Cassandra liveness check failed, due to %v\n", err)
		}
		log.Info("[Confirmation]: The cassandra application liveness pod deployed successfully")
	} else {
		log.Info("[Liveness]: Cassandra Liveness check skipped as it was not enabled")
	}

	// Including the litmus lib for cassandra-pod-delete
	if experimentsDetails.ChaosLib == "litmus" {
		err = pod_delete.PreparePodDelete(&experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails)
		if err != nil {
			log.Errorf("Chaos injection failed due to %v\n", err)
			failStep := "Including the litmus lib for pod-delete"
			types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
			result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
			return
		}
		log.Info("[Confirmation]: The application pod has been deleted successfully")
		resultDetails.Verdict = "Pass"
	} else {
		log.Error("[Invalid]: Please Provide the correct LIB")
		failStep := "Including the litmus lib for pod-delete"
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
	if experimentsDetails.EngineName != "" {
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, "AUT is Running successfully", "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// Checking the load distribution on the ring (post-chaos)
	log.Info("[Status]: Checking the load distribution on the ring (post-chaos)")
	err = cassandra.NodeToolStatusCheck(&experimentsDetails, clients)
	if err != nil {
		log.Fatalf("[Status]: Chaos node tool status check is failed due to %v\n", err)
	}

	// Cassandra statefulset liveness check (post-chaos)
	log.Info("[Status]: Confirm that the cassandra liveness pod is running(post-chaos)")
	// Checking the running status of cassandra liveness
	if experimentsDetails.CassandraLivenessCheck == "enabled" {
		err = status.CheckApplicationStatus(experimentsDetails.AppNS, "name=cassandra-liveness-deploy-"+experimentsDetails.RunID, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			log.Fatalf("Liveness status check failed due to %v\n", err)
		}
		err = cassandra.LivenessCleanup(&experimentsDetails, clients, ResourceVersionBefore)
		if err != nil {
			log.Fatalf("Liveness cleanup failed due to %v\n", err)
		}
	}
	//Updating the chaosResult in the end of experiment
	log.Info("[The End]: Updating the chaos result of cassandra pod delete experiment experiment (EOT)")
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
