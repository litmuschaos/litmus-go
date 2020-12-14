package experiment

import (
	kafka_broker_pod_failure "github.com/litmuschaos/litmus-go/chaoslib/litmus/kafka-broker-pod-failure/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/kafka"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/kafka/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
)

// KafkaBrokerPodFailure inject the kafka-broker-pod-failure chaos
func KafkaBrokerPodFailure(clients clients.ClientSets) {

	var err error
	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}

	//Fetching all the ENV passed from the runner pod
	log.Info("[PreReq]: Getting the ENV for the kafka-broker-pod-failure")
	experimentEnv.GetENV(&experimentsDetails)

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		// Intialise the probe details. Bail out upon error, as we haven't entered exp business logic yet
		if err = probe.InitializeProbesInChaosResultDetails(&chaosDetails, clients, &resultDetails); err != nil {
			log.Fatalf("Unable to initialize the probes, err: %v", err)
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ChaoslibDetail.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result, err: %v", err)
		failStep := "Updating the chaos result of kafka-broker-pod-failure experiment (SOT)"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	// generating the event in chaosresult to marked the verdict as awaited
	msg := "experiment: " + experimentsDetails.ChaoslibDetail.ExperimentName + ", Result: Awaited"
	types.SetResultEventAttributes(&eventsDetails, types.AwaitedVerdict, msg, "Normal", &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	//DISPLAY THE APP INFORMATION
	log.InfoWithValues("The application informations are as follows", logrus.Fields{
		"Kafka Namespace": experimentsDetails.KafkaNamespace,
		"Kafka Label":     experimentsDetails.KafkaLabel,
		"Ramp Time":       experimentsDetails.ChaoslibDetail.RampTime,
	})

	// PRE-CHAOS APPLICATION STATUS CHECK
	// KAFKA CLUSTER HEALTH CHECK
	log.Info("[Status]: Verify that the Kafka cluster is healthy(pre-chaos)")
	err = kafka.ClusterHealthCheck(&experimentsDetails, clients)
	if err != nil {
		log.Errorf("Cluster status check failed, err: %v", err)
		failStep := "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}
	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the pre-chaos check
		if len(resultDetails.ProbeDetails) != 0 {

			err = probe.RunProbes(&chaosDetails, clients, &resultDetails, "PreChaos", &eventsDetails)
			if err != nil {
				log.Errorf("Probes Failed, err: %v", err)
				failStep := "Failed while running probes"
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Warning", &chaosDetails)
				events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
				result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}
		// generating the events for the pre-chaos check
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// PRE-CHAOS KAFKA APPLICATION LIVENESS CHECK
	// when the kafka broker is provided
	if experimentsDetails.KafkaBroker != "" {
		if experimentsDetails.KafkaLivenessStream == "enabled" {
			_, err := kafka.LivenessStream(&experimentsDetails, clients)
			if err != nil {
				log.Errorf("Liveness check failed, err: %v", err)
				failStep := "Verify liveness check (pre-chaos)"
				types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
				result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
				return

			}
			log.Info("The Liveness pod gets established")

		} else if experimentsDetails.KafkaLivenessStream == "" || experimentsDetails.KafkaLivenessStream == "disabled" {
			kafka.DisplayKafkaBroker(&experimentsDetails)
		}
	}

	// when the kafka broker is not provided
	if experimentsDetails.KafkaBroker == "" {
		if experimentsDetails.KafkaLivenessStream == "enabled" {
			err = kafka.LaunchStreamDeriveLeader(&experimentsDetails, clients)
			if err != nil {
				log.Errorf("Error: %v", err)
				failStep := "Launch the stream derive leader"
				types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
				result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
				return
			}
		} else if experimentsDetails.KafkaLivenessStream == "" || experimentsDetails.KafkaLivenessStream == "disabled" {
			_, err := kafka.SelectBroker(&experimentsDetails, "", clients)
			if err != nil {
				log.Errorf("Error: %v", err)
				failStep := "Selecting broker when liveness is disabled"
				types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
				result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
				return
			}
		}
	}

	// Including the litmus lib for kafka-broker-pod-failure
	if experimentsDetails.ChaoslibDetail.ChaosLib == "litmus" {
		err = kafka_broker_pod_failure.PreparePodDelete(&experimentsDetails, experimentsDetails.ChaoslibDetail, clients, &resultDetails, &eventsDetails, &chaosDetails)
		if err != nil {
			log.Errorf("Chaos injection failed, err: %v", err)
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
		log.Errorf("Cluster status check failed, err: %v", err)
		failStep := "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
		result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
		return
	}
	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			err = probe.RunProbes(&chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails)
			if err != nil {
				log.Errorf("Probe Failed, err: %v", err)
				failStep := "Failed while running probes"
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Warning", &chaosDetails)
				events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
				result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}

		// generating post chaos event
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// Liveness Status Check (post-chaos) and cleanup
	if experimentsDetails.KafkaLivenessStream != "" {
		err = status.CheckApplicationStatus(experimentsDetails.ChaoslibDetail.AppNS, "name=kafka-liveness", experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients)
		if err != nil {
			log.Errorf("Application liveness check failed, err: %v", err)
			failStep := "Verify that the AUT (Application Under Test) is running (post-chaos)"
			types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
			result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
			return
		}

		if err := kafka.LivenessCleanup(&experimentsDetails, clients); err != nil {
			log.Errorf("liveness cleanup failed, err: %v", err)
			failStep := "Performing cleanup post chaos"
			types.SetResultAfterCompletion(&resultDetails, "Fail", "Completed", failStep)
			result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
			return
		}

	}

	//Updating the chaosResult in the end of experiment
	log.Info("[The End]: Updating the chaos result of kafka pod delete experiment (EOT)")
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
	if err != nil {
		log.Errorf("Unable to Update the Chaos Result, err: %v", err)
	}

	// generating the event in chaosresult to marked the verdict as pass/fail
	msg = "experiment: " + experimentsDetails.ChaoslibDetail.ExperimentName + ", Result: " + resultDetails.Verdict
	reason := types.PassVerdict
	eventType := "Normal"
	if resultDetails.Verdict != "Pass" {
		reason = types.FailVerdict
		eventType = "Warning"
	}

	types.SetResultEventAttributes(&eventsDetails, reason, msg, eventType, &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		msg := experimentsDetails.ChaoslibDetail.ExperimentName + " experiment has been " + resultDetails.Verdict + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}
}
