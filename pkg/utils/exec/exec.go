package exec

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/remotecommand"
)

// CommandDetails contains all the required variables for exec inside a container
type CommandDetails struct {
	PodName       string
	Namespace     string
	ContainerName string
	Command       string
}

// Exec function will run the given commands inside the provided container
func Exec(commandDetails *CommandDetails, clients clients.ClientSets) (string, error) {

	req := clients.KubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(commandDetails.PodName).
		Namespace(commandDetails.Namespace).
		SubResource("exec")
	scheme := runtime.NewScheme()
	if err := apiv1.AddToScheme(scheme); err != nil {
		return "", fmt.Errorf("error adding to scheme: %v", err)
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&apiv1.PodExecOptions{
		Command:   strings.Fields(commandDetails.Command),
		Container: commandDetails.ContainerName,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(clients.KubeConfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("error while creating Executor: %v", err)
	}

	var out bytes.Buffer
	stdout := &out
	stderr := os.Stderr

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})

	if err != nil {
		errorCode := strings.Contains(err.Error(), "143")
		if errorCode != true {
			log.Infof("[Prepare]: Unable to run command inside container due to, err : %v", err.Error())
			return "", err
		}
	}

	return out.String(), nil
}

//SetExecCommandAttributes initialise all the chaos result ENV
func SetExecCommandAttributes(commandDetails *CommandDetails, PodName, ContainerName, Namespace, Command string) {

	commandDetails.Command = Command
	commandDetails.ContainerName = ContainerName
	commandDetails.Namespace = Namespace
	commandDetails.PodName = PodName
}
