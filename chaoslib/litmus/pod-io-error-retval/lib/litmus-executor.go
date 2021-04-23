package lib

import (
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	litmusexec "github.com/litmuschaos/litmus-go/pkg/utils/exec"
)

// LitmusExecutor implements the Executor interface to executing commands on pods
type LitmusExecutor struct {
	ContainerName string
	PodName       string
	Namespace     string
	Clients       clients.ClientSets
}

// Execute executes commands on the current container in the current pod
func (executor LitmusExecutor) Execute(command Command) error {
	log.Infof("Executing command: $%s", command)

	execCommandDetails := litmusexec.PodDetails{}
	cmd := []string{"/bin/sh", "-c", string(command)}

	litmusexec.SetExecCommandAttributes(
		&execCommandDetails,
		executor.PodName,
		executor.ContainerName,
		executor.Namespace,
	)

	_, err := litmusexec.Exec(
		&execCommandDetails, executor.Clients, cmd)

	return err
}
