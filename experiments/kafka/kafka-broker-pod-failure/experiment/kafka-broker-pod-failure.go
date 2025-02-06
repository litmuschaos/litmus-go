package experiment

import (
	"context"
	"os"
	"strings"

	"github.com/litmuschaos/chaos-operator/api/litmuschaos/v1alpha1"
	kafkaPodDelete "github.com/litmuschaos/litmus-go/chaoslib/litmus/kafka-broker-pod-failure/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/kafka"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/kafka/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kafka/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// KafkaBrokerPodFailure derive and kill the kafka broker leader
func KafkaBrokerPodFailure(ctx context.Context, clients clients.ClientSets) {
	span := trace.SpanFromContext(ctx)

	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}

	//Fetching all the ENV passed from the runner pod
	log.Infof("[PreReq]: Getting the ENV for the %v experiment", os.Getenv("EXPERIMENT_NAME"))
	experimentEnv.GetENV(&experimentsDetails)

	// Initialize the chaos attributes
	types.InitialiseChaosVariables(&chaosDetails)

	// Initialize Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		// Get values from chaosengine. Bail out upon error, as we haven't entered exp business logic yet
		if err := types.GetValuesFromChaosEngine(&chaosDetails, clients, &resultDetails); err != nil {
			log.Errorf("Unable to initialize the probes, err: %v", err)
			span.SetStatus(codes.Error, "Unable to initialize the probes")
			span.RecordError(err)
			return
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ChaoslibDetail.ExperimentName)
	if err := result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT"); err != nil {
		log.Errorf("Unable to Create the Chaos Result, err: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Unable to Create the Chaos Resultt")
		span.RecordError(err)
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
		"Chaos Duration":  experimentsDetails.ChaoslibDetail.ChaosDuration,
	})

	// PRE-CHAOS APPLICATION STATUS CHECK
	// KAFKA CLUSTER HEALTH CHECK
	if chaosDetails.DefaultHealthCheck {
		log.Info("[Status]: Verify that the Kafka cluster is healthy(pre-chaos)")
		if err := kafka.ClusterHealthCheck(&experimentsDetails, clients); err != nil {
			log.Errorf("Cluster health check failed, err: %v", err)
			types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, "AUT: Not Running", "Warning", &chaosDetails)
			events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "Cluster health check failed")
			span.RecordError(err)
			return
		}
	}

	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the pre-chaos check
		if len(resultDetails.ProbeDetails) != 0 {

			if err := probe.RunProbes(ctx, &chaosDetails, clients, &resultDetails, "PreChaos", &eventsDetails); err != nil {
				log.Errorf("Probes Failed, err: %v", err)
				msg := "AUT: Running, Probes: Unsuccessful"
				types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Warning", &chaosDetails)
				events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
				span.SetStatus(codes.Error, "Probes Failed")
				span.RecordError(err)
				return
			}
			msg = "AUT: Running, Probes: Successful"
		}
		// generating the events for the pre-chaos check
		types.SetEngineEventAttributes(&eventsDetails, types.PreChaosCheck, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// PRE-CHAOS KAFKA APPLICATION LIVENESS CHECK
	switch strings.ToLower(experimentsDetails.KafkaLivenessStream) {
	case "enable":
		livenessTopicLeader, err := kafka.LivenessStream(&experimentsDetails, clients)
		if err != nil {
			log.Errorf("Liveness check failed, err: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "Liveness check failed")
			span.RecordError(err)
			return
		}
		log.Info("The Liveness pod gets established")
		log.Infof("[Info]: Kafka partition leader is %v", livenessTopicLeader)

		if experimentsDetails.KafkaBroker == "" {
			experimentsDetails.KafkaBroker = livenessTopicLeader
		}
	}

	kafka.DisplayKafkaBroker(&experimentsDetails)

	chaosDetails.Phase = types.ChaosInjectPhase

	if err := kafkaPodDelete.PreparePodDelete(ctx, &experimentsDetails, clients, &resultDetails, &eventsDetails, &chaosDetails); err != nil {
		log.Errorf("Chaos injection failed, err: %v", err)
		result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
		span.SetStatus(codes.Error, "Chaos injection failed")
		span.RecordError(err)
		return
	}

	log.Infof("[Confirmation]: %v chaos has been injected successfully", experimentsDetails.ExperimentName)
	resultDetails.Verdict = v1alpha1.ResultVerdictPassed

	chaosDetails.Phase = types.PostChaosPhase

	// POST-CHAOS KAFKA CLUSTER HEALTH CHECK
	if chaosDetails.DefaultHealthCheck {
		log.Info("[Status]: Verify that the Kafka cluster is healthy(post-chaos)")
		if err := kafka.ClusterHealthCheck(&experimentsDetails, clients); err != nil {
			log.Errorf("Cluster health check failed, err: %v", err)
			types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, "AUT: Not Running", "Warning", &chaosDetails)
			events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "Cluster health check failed")
			span.RecordError(err)
			return
		}
	}

	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := common.GetStatusMessage(chaosDetails.DefaultHealthCheck, "AUT: Running", "")

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			if err := probe.RunProbes(ctx, &chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails); err != nil {
				log.Errorf("Probe Failed, err: %v", err)
				msg := common.GetStatusMessage(chaosDetails.DefaultHealthCheck, "AUT: Running", "Unsuccessful")
				types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Warning", &chaosDetails)
				events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
				result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
				span.SetStatus(codes.Error, "Probe Failed")
				span.RecordError(err)
				return
			}
			msg = common.GetStatusMessage(chaosDetails.DefaultHealthCheck, "AUT: Running", "Successful")
		}

		// generating post chaos event
		types.SetEngineEventAttributes(&eventsDetails, types.PostChaosCheck, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}

	// Liveness Status Check (post-chaos) and cleanup
	switch strings.ToLower(experimentsDetails.KafkaLivenessStream) {
	case "enable":
		log.Info("[Status]: Verify that the Kafka liveness pod is running(post-chaos)")
		if err := status.CheckApplicationStatusesByLabels(experimentsDetails.ChaoslibDetail.AppNS, "name=kafka-liveness-"+experimentsDetails.RunID, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients); err != nil {
			log.Errorf("Application liveness status check failed, err: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "Application liveness status check failed")
			span.RecordError(err)
			return
		}

		log.Info("[CleanUp]: Deleting the kafka liveness pod(post-chaos)")
		if err := kafka.LivenessCleanup(&experimentsDetails, clients); err != nil {
			log.Errorf("liveness cleanup failed, err: %v", err)
			result.RecordAfterFailure(&chaosDetails, &resultDetails, err, clients, &eventsDetails)
			span.SetStatus(codes.Error, "liveness cleanup failed")
			span.RecordError(err)
			return
		}
	}

	//Updating the chaosResult in the end of experiment
	log.Info("[The End]: Updating the chaos result of kafka pod delete experiment (EOT)")
	if err := result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT"); err != nil {
		log.Errorf("Unable to Update the Chaos Result, err: %v", err)
		span.SetStatus(codes.Error, "Unable to Update the Chaos Result")
		span.RecordError(err)
		return
	}

	// generating the event in chaosresult to marked the verdict as pass/fail
	msg = "experiment: " + experimentsDetails.ChaoslibDetail.ExperimentName + ", Result: " + string(resultDetails.Verdict)
	reason, eventType := types.GetChaosResultVerdictEvent(resultDetails.Verdict)

	types.SetResultEventAttributes(&eventsDetails, reason, msg, eventType, &resultDetails)
	events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosResult")

	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		msg := experimentsDetails.ChaoslibDetail.ExperimentName + " experiment has been " + string(resultDetails.Verdict) + "ed"
		types.SetEngineEventAttributes(&eventsDetails, types.Summary, msg, "Normal", &chaosDetails)
		events.GenerateEvents(&eventsDetails, clients, &chaosDetails, "ChaosEngine")
	}
}
