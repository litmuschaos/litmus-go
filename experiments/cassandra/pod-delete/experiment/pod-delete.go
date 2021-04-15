package experiment

import (
	litmusLIB "github.com/litmuschaos/litmus-go/chaoslib/litmus/pod-delete/lib"
	"github.com/litmuschaos/litmus-go/pkg/cassandra"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/cassandra/pod-delete/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/cassandra/pod-delete/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
)

// CasssandraPodDelete inject the cassandra-pod-delete chaos
func CasssandraPodDelete(clients clients.ClientSets) {

	var err error
	var ResourceVersionBefore string
	experimentsDetails := experimentTypes.ExperimentDetails{}
	resultDetails := types.ResultDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}

	//Fetching all the ENV passed from the runner pod
	log.Info("[PreReq]: Getting the ENV for the cassandra-pod-delete experiment")
	experimentEnv.GetENV(&experimentsDetails)

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		// Intialise the probe details. Bail out upon error, as we haven't entered exp business logic yet
		if err = probe.InitializeProbesInChaosResultDetails(&chaosDetails, clients, &resultDetails); err != nil {
			log.Errorf("Unable to initialize the probes, err: %v", err)
			return
		}
	}

	//Updating the chaos result in the beginning of experiment
	log.Infof("[PreReq]: Updating the chaos result of %v experiment (SOT)", experimentsDetails.ChaoslibDetail.ExperimentName)
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "SOT")
	if err != nil {
		log.Errorf("Unable to Create the Chaos Result, err: %v", err)
		failStep := "Updating the chaos result of pod-delete experiment (SOT)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
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
		"Namespace":              experimentsDetails.ChaoslibDetail.AppNS,
		"Label":                  experimentsDetails.ChaoslibDetail.AppLabel,
		"CassandraLivenessImage": experimentsDetails.CassandraLivenessImage,
		"CassandraLivenessCheck": experimentsDetails.CassandraLivenessCheck,
		"CassandraPort":          experimentsDetails.CassandraPort,
		"Ramp Time":              experimentsDetails.ChaoslibDetail.RampTime,
	})

	//PRE-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (pre-chaos)")
	if err = status.AUTStatusCheck(experimentsDetails.ChaoslibDetail.AppNS, experimentsDetails.ChaoslibDetail.AppLabel, experimentsDetails.ChaoslibDetail.TargetContainer, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients, &chaosDetails); err != nil {
		log.Errorf("Application status check failed, err: %v", err)
		failStep := "Verify that the AUT (Application Under Test) is running (pre-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
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

	// Checking the load distribution on the ring (pre-chaos)
	log.Info("[Status]: Checking the load distribution on the ring (pre-chaos)")
	err = cassandra.NodeToolStatusCheck(&experimentsDetails, clients)
	if err != nil {
		log.Errorf("[Status]: Chaos node tool status check failed, err: %v", err)
		failStep := "Checking for load distribution on the ring(pre-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	// Cassandra liveness check
	if experimentsDetails.CassandraLivenessCheck == "enabled" {
		ResourceVersionBefore, err = cassandra.LivenessCheck(&experimentsDetails, clients)
		if err != nil {
			log.Errorf("[Liveness]: Cassandra liveness check failed, err: %v", err)
			failStep := "failed while creating liveness pod"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
		log.Info("[Confirmation]: The cassandra application liveness pod created successfully")
	} else {
		log.Warn("[Liveness]: Cassandra Liveness check skipped as it was not enabled")
	}

	// Including the litmus lib for cassandra-pod-delete
	if experimentsDetails.ChaoslibDetail.ChaosLib == "litmus" {
		err = litmusLIB.PreparePodDelete(experimentsDetails.ChaoslibDetail, clients, &resultDetails, &eventsDetails, &chaosDetails)
		if err != nil {
			log.Errorf("Chaos injection failed, err: %v", err)
			failStep := "failed in chaos injection phase"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
		log.Info("[Confirmation]: The application pod has been deleted successfully")
		resultDetails.Verdict = "Pass"
	} else {
		log.Error("[Invalid]: Please Provide the correct LIB")
		failStep := "no match found for specified lib"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	//POST-CHAOS APPLICATION STATUS CHECK
	log.Info("[Status]: Verify that the AUT (Application Under Test) is running (post-chaos)")
	if err = status.AUTStatusCheck(experimentsDetails.ChaoslibDetail.AppNS, experimentsDetails.ChaoslibDetail.AppLabel, experimentsDetails.ChaoslibDetail.TargetContainer, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients, &chaosDetails); err != nil {
		log.Errorf("Application status check failed, err: %v", err)
		failStep := "Verify that the AUT (Application Under Test) is running (post-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}
	if experimentsDetails.ChaoslibDetail.EngineName != "" {
		// marking AUT as running, as we already checked the status of application under test
		msg := "AUT: Running"

		// run the probes in the post-chaos check
		if len(resultDetails.ProbeDetails) != 0 {
			err = probe.RunProbes(&chaosDetails, clients, &resultDetails, "PostChaos", &eventsDetails)
			if err != nil {
				log.Errorf("Probes Failed, err: %v", err)
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

	// Checking the load distribution on the ring (post-chaos)
	log.Info("[Status]: Checking the load distribution on the ring (post-chaos)")
	err = cassandra.NodeToolStatusCheck(&experimentsDetails, clients)
	if err != nil {
		log.Errorf("[Status]: Chaos node tool status check is failed, err: %v", err)
		failStep := "Checking for load distribution on the ring(post-chaos)"
		result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
		return
	}

	// Cassandra statefulset liveness check (post-chaos)
	log.Info("[Status]: Confirm that the cassandra liveness pod is running(post-chaos)")
	// Checking the running status of cassandra liveness
	if experimentsDetails.CassandraLivenessCheck == "enabled" {
		err = status.CheckApplicationStatus(experimentsDetails.ChaoslibDetail.AppNS, "name=cassandra-liveness-deploy-"+experimentsDetails.RunID, experimentsDetails.ChaoslibDetail.Timeout, experimentsDetails.ChaoslibDetail.Delay, clients)
		if err != nil {
			log.Errorf("Liveness status check failed, err: %v", err)
			failStep := "failed while checking the status of liveness pod"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
		err = cassandra.LivenessCleanup(&experimentsDetails, clients, ResourceVersionBefore)
		if err != nil {
			log.Errorf("Liveness cleanup failed, err: %v", err)
			failStep := "failed while deleting liveness pod"
			result.RecordAfterFailure(&chaosDetails, &resultDetails, failStep, clients, &eventsDetails)
			return
		}
	}
	//Updating the chaosResult in the end of experiment
	log.Info("[The End]: Updating the chaos result of cassandra pod delete experiment (EOT)")
	err = result.ChaosResult(&chaosDetails, clients, &resultDetails, "EOT")
	if err != nil {
		log.Errorf("Unable to Update the Chaos Result, err: %v", err)
		return
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
