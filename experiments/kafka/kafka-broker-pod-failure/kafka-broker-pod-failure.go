package main

import (
	"fmt"

	pod_delete "github.com/litmuschaos/litmus-go/chaoslib/litmus/pod-delete/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/kafka"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/kafka/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
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
	log.Info("[PreReq]: Getting the ENV for the kafka-broker-pod-failure")
	experimentEnv.GetENV(&experimentsDetails)

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)
	fmt.Printf(experimentsDetails.ChaoslibDetail.ChaosLib)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ChaoslibDetail.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result due to %v", err)
		failStep := "Updating the chaos result of kafka-broker-pod-failure experiment (SOT)"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("The application informations are as follows", logrus.Fields{
		"Kafka Namespace": experimentsDetails.KafkaNamespace,
		"Kafka Label":     experimentsDetails.KafkaLabel,
		"Ramp Time":       experimentsDetails.ChaoslibDetail.RampTime,
	})

	// PRE-CHAOS KAFKA CLUSTER HEALTH CHECK
	log.Info("[Status]: Verify that the Kafka cluster is healthy(pre-chaos)")
	err = kafka.ClusterHealthCheck(&experimentsDetails, clients)
	if err != nil {
		log.Errorf("Cluster status check failed due to %v\n", err)
		failStep := "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}
	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, "AUT is Running successfully", "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// PRE-CHAOS KAFKA APPLICATION LIVENESS CHECK
	if experimentsDetails.KafkaBroker != "" {
		if experimentsDetails.KafkaLivenessStream == "enabled" {
			_, err := kafka.LivenessStream(&experimentsDetails, clients)
			if err != nil {
				log.Fatalf("Liveness check failed, due to %v", err)
			}
			log.Info("The Liveness pod gets established")

		} else if experimentsDetails.KafkaLivenessStream == "" || experimentsDetails.KafkaLivenessStream == "disabled" {
			kafka.DisplayKafkaBrocker(&experimentsDetails)
		}
	}

	if experimentsDetails.KafkaBroker == "" {
		if experimentsDetails.KafkaLivenessStream == "enabled" {
			err = kafka.LaunchStreamDeriveLeader(&experimentsDetails, clients)
			if err != nil {
				log.Fatalf("Error: %v", err)
			}
		} else if experimentsDetails.KafkaLivenessStream == "" || experimentsDetails.KafkaLivenessStream == "disabled" {
			_, err := kafka.SelectBroker(&experimentsDetails, "", clients)
			if err != nil {
				log.Fatalf("Error: %v", err)
			}
		}
	}

	// Including the litmus lib for kafka-broker-pod-failure
	experimentsDetails.ChaoslibDetail.TargetPod = experimentsDetails.KafkaBroker
	if experimentsDetails.ChaoslibDetail.ChaosLib == "litmus" {
		err = pod_delete.PreparePodDelete(experimentsDetails.ChaoslibDetail, clients, &resultDetails, &eventsDetails, &chaosDetails)
		if err != nil {
			log.Errorf("Chaos injection failed due to %v\n", err)
			failStep := "Including the litmus lib for kafka-broker-pod-failure"
			types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
			result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
			return
		}
		log.Info("[Confirmation]: The application pod has been deleted successfully")
		resultDetails.Verdict = "Pass"
	} else {
		log.Error("[Invalid]: Please Provide the correct LIB")
		failStep := "Including the litmus lib for kafka-broker-pod-failure"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	// POST-CHAOS KAFKA CLUSTER HEALTH CHECK
	log.Info("[Status]: Verify that the Kafka cluster is healthy(post-chaos)")
	err = kafka.ClusterHealthCheck(&experimentsDetails, clients)
	if err != nil {
		log.Errorf("Cluster status check failed due to %v\n", err)
		failStep := "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}
	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, "AUT is Running successfully", "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// Liveness Status Check (post-chaos) and cleanup
	if experimentsDetails.KafkaLivenessStream != "" {
		err = status.CheckApplicationStatus(experimentsDetails.ChaoslibDetail.AppNS, "name=kafka-liveness", experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients)
		if err != nil {
			log.Fatalf("Application liveness check failed due to %v\n", err)
		}
		if err := kafka.LivenessCleanup(&experimentsDetails, clients); err != nil {
			log.Fatalf("Error in liveness cleanup: %v", err)
		}

	}

	//Updating the chaosResult in the end of experiment
	log.Info("[The End]: Updating the chaos result of cassandra pod delete experiment (EOT)")
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
	if err != nil {
		log.Fatalf("Unable to Update the Chaos Result due to %v\n", err)
	}
	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		msg := experimentsDetails.ChaoslibDetail.ExperimentName + " experiment has been " + resultDetails.Verdict + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	msg := experimentsDetails.ChaoslibDetail.ExperimentName + " experiment has been " + resultDetails.Verdict + "ed"
	types.SetResultEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")
}
