package utils

import (
	"os"
	"strconv"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = expName
	experimentDetails.ChaosNamespace = "shubham"
	experimentDetails.EngineName = "engine"
	experimentDetails.ChaosDuration = 30
	experimentDetails.ChaosInterval = 10
	experimentDetails.RampTime = 10
	experimentDetails.Force, _ = strconv.ParseBool(os.Getenv("FORCE"))
	experimentDetails.ChaosLib = "litmus"
	experimentDetails.ChaosServiceAccount = os.Getenv("CHAOS_SERVICE_ACCOUNT")
	experimentDetails.AppNS = "shubham"
	experimentDetails.AppLabel = "run=nginx"
	experimentDetails.AppKind = os.Getenv("APP_KIND")
	experimentDetails.KillCount = 5
	experimentDetails.ChaosUID = os.Getenv("CHAOS_UID")
	experimentDetails.AuxiliaryAppInfo = ""
	experimentDetails.InstanceID = "12345"
}
