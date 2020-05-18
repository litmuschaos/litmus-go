package environment

import (
	"os"
	"strconv"
	types "github.com/litmuschaos/litmus-go/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *types.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = expName
	experimentDetails.ChaosNamespace = "test"
	experimentDetails.EngineName = ""
	experimentDetails.ChaosDuration = 10
	experimentDetails.ChaosInterval= 10
	experimentDetails.RampTime,_ = strconv.Atoi(os.Getenv("RAMP_TIME"))
	experimentDetails.ChaosLib = "litmus"
	experimentDetails.ChaosServiceAccount = "litmus"
	experimentDetails.AppNS = "test"
	experimentDetails.AppLabel = "run=nginx"
	experimentDetails.AppKind = os.Getenv("APP_KIND")
	experimentDetails.KillCount,_ =  strconv.Atoi(os.Getenv("KILL_COUNT"))
	experimentDetails.ChaosUID = os.Getenv("CHAOS_UID")
	experimentDetails.AuxiliaryAppInfo = ""
	experimentDetails.InstanceID = os.Getenv("INSTANCE_ID")
	experimentDetails.ChaosPodName = "pod-delete"
}

//SetResultAttributes initialise all the chaos result ENV
func SetResultAttributes(resultDetails *types.ResultDetails, experimentDetails *types.ExperimentDetails) {
	resultDetails.Verdict = "Awaited"
	resultDetails.Phase = "Running"
	resultDetails.FailStep = "N/A"
	if experimentDetails.EngineName != ""{
	resultDetails.Name = experimentDetails.EngineName+"-"+experimentDetails.ExperimentName
	} else {
		resultDetails.Name = experimentDetails.ExperimentName
	}
}
