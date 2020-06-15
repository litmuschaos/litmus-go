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
	experimentDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE","litmus")
	experimentDetails.EngineName = Getenv("CHAOSENGINE","")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION","30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(Getenv("CHAOS_INTERVAL","10"))
	experimentDetails.RampTime, _ = strconv.Atoi(Getenv("RAMP_TIME","0"))
	experimentDetails.ChaosLib = Getenv("LIB","litmus")
	experimentDetails.ChaosServiceAccount = Getenv("CHAOS_SERVICE_ACCOUNT","")
	experimentDetails.AppNS = Getenv("APP_NAMESPACE","")
	experimentDetails.AppLabel = Getenv("APP_LABEL","")
	experimentDetails.AppKind = Getenv("APP_KIND","")
	experimentDetails.KillCount, _ = strconv.Atoi(Getenv("KILL_COUNT","1"))
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID",""))
	experimentDetails.AuxiliaryAppInfo = Getenv("AUXILIARY_APPINFO","")
	experimentDetails.InstanceID = Getenv("INSTANCE_ID","")
	experimentDetails.ChaosPodName = Getenv("POD_NAME","")
	experimentDetails.Force, _ = strconv.ParseBool(Getenv("FORCE","false"))
	experimentDetails.LIBImage = Getenv("LIB_IMAGE","")
  	experimentDetails.CPUcores, _ = strconv.Atoi(Getenv("CPU_CORES","1"))
	experimentDetails.PodsAffectedPerc, _ = strconv.Atoi(Getenv("PODS_AFFECTED_PERC","100"))
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

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string{
	value := os.Getenv(key)
	if value == ""{
		value = defaultValue
	}
	return value
}
