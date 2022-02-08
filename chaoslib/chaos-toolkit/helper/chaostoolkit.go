package helper

import (
	"bytes"
	"os/exec"
	"strconv"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/chaos-toolkit/types"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

// Helper injects the container-kill chaos
func Helper(clients clients.ClientSets) {

	experimentsDetails := experimentTypes.ExperimentDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}
	resultDetails := types.ResultDetails{}

	//Fetching all the ENV passed in the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	getENV(&experimentsDetails)

	// Intialise the chaos attributes
	types.InitialiseChaosVariables(&chaosDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	err := execChaosToolkit(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails)
	if err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}
}

// killContainer kill the random application container
// it will kill the container till the chaos duration
// the execution will stop after timestamp passes the given chaos duration
func execChaosToolkit(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"AppName":   experimentsDetails.AppName,
			"OrgName":   experimentsDetails.OrgName,
			"SpaceName": experimentsDetails.SpaceName,
		})

		// record the event inside chaosengine
		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		if err := execChaosToolkitCmd(experimentsDetails.ExperimentName); err != nil {
			return err
		}
		//Waiting for the chaos interval after chaos injection
		if experimentsDetails.ChaosInterval != 0 {
			log.Infof("[Wait]: Wait for the chaos interval %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	if err := result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "targeted", "pod", experimentsDetails.AppName); err != nil {
		return err
	}
	log.Infof("[Completion]: %v chaos has been completed", experimentsDetails.ExperimentName)
	return nil
}

//stopDockerContainer kill the application container
func execChaosToolkitCmd(experimentName string) error {
	var errOut bytes.Buffer
	cmd := exec.Command("chaos", "run", experimentName+".json")
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return errors.Errorf("Unable to run command, err: %v; error output: %v", err, errOut.String())
	}
	return nil
}

//getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(types.Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "30"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.ChaosNamespace = types.Getenv("INSTANCE_ID", "")
	experimentDetails.AppName = types.Getenv("APP_NAME", "")
	experimentDetails.OrgName = types.Getenv("ORG_NAME", "")
	experimentDetails.SpaceName = types.Getenv("SPACE_NAME", "")
}
