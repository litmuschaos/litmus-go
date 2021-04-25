package experiment

import (
	litmusLib "github.com/litmuschaos/litmus-go/chaoslib/litmus/pod-io-error-retval/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
)

func PodIoErrorRetval(clients clients.ClientSets) {
	PodChaosExperiment(clients, litmusLib.FailFunctionLitmusChaosInjector())
}
