package environment

import (
	"os"
	"strconv"

	clientTypes "k8s.io/apimachinery/pkg/types"

	types "github.com/litmuschaos/litmus-go/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *types.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = expName
	experimentDetails.ChaosNamespace = os.Getenv("CHAOS_NAMESPACE")
	experimentDetails.EngineName = os.Getenv("CHAOSENGINE")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(os.Getenv("TOTAL_CHAOS_DURATION"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(os.Getenv("CHAOS_INTERVAL"))
	experimentDetails.RampTime, _ = strconv.Atoi(os.Getenv("RAMP_TIME"))
	experimentDetails.ChaosLib = os.Getenv("LIB")
	experimentDetails.ChaosServiceAccount = os.Getenv("CHAOS_SERVICE_ACCOUNT")
	experimentDetails.AppNS = os.Getenv("APP_NAMESPACE")
	experimentDetails.AppLabel = os.Getenv("APP_LABEL")
	experimentDetails.AppKind = os.Getenv("APP_KIND")
	experimentDetails.KillCount, _ = strconv.Atoi(os.Getenv("KILL_COUNT"))
	experimentDetails.ChaosUID = clientTypes.UID(os.Getenv("CHAOS_UID"))
	experimentDetails.AuxiliaryAppInfo = os.Getenv("AUXILIARY_APPINFO")
	experimentDetails.InstanceID = os.Getenv("INSTANCE_ID")
	experimentDetails.ChaosPodName = os.Getenv("POD_NAME")
	experimentDetails.Force, _ = strconv.ParseBool(os.Getenv("FORCE"))
}

//SetResultAttributes initialise all the chaos result ENV
func SetResultAttributes(resultDetails *types.ResultDetails, experimentDetails *types.ExperimentDetails) {
	resultDetails.Verdict = "Awaited"
	resultDetails.Phase = "Running"
	resultDetails.FailStep = "N/A"
	if experimentDetails.EngineName != "" {
		resultDetails.Name = experimentDetails.EngineName + "-" + experimentDetails.ExperimentName
	} else {
		resultDetails.Name = experimentDetails.ExperimentName
	}
}

//SetEventAttributes initialise all the chaos result ENV
func SetEventAttributes(eventsDetails *types.EventDetails, Reason string, Message string) {

	eventsDetails.Reason = Reason
	eventsDetails.Message = Message
}
