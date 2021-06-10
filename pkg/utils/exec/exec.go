package exec

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/tools/remotecommand"
)

// PodDetails contains all the required variables to exec inside a container
type PodDetails struct {
	PodName       string
	Namespace     string
	ContainerName string
}

// Exec function will run the provide commands inside the target container
func Exec(commandDetails *PodDetails, clients clients.ClientSets, command []string) (string, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(commandDetails.Namespace).Get(commandDetails.PodName, v1.GetOptions{})
	if err != nil {
		return "", errors.Errorf("unable to get %v pod in %v namespace, err: %v", commandDetails.PodName, commandDetails.Namespace, err)
	}
	if err := checkPodStatus(pod, commandDetails.ContainerName); err != nil {
		return "", err
	}

	req := clients.KubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(commandDetails.PodName).
		Namespace(commandDetails.Namespace).
		SubResource("exec")
	scheme := runtime.NewScheme()
	if err := apiv1.AddToScheme(scheme); err != nil {
		return "", fmt.Errorf("error adding to scheme: %v", err)
	}

	// NewParameterCodec creates a ParameterCodec capable of transforming url values into versioned objects and back.
	parameterCodec := runtime.NewParameterCodec(scheme)

	req.VersionedParams(&apiv1.PodExecOptions{
		Command:   command,
		Container: commandDetails.ContainerName,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	// NewSPDYExecutor connects to the provided server and upgrades the connection to
	// multiplexed bidirectional streams.
	exec, err := remotecommand.NewSPDYExecutor(clients.KubeConfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("error while creating Executor: %v", err)
	}

	// storing the output inside the output buffer for future use
	var out bytes.Buffer
	stdout := &out
	stderr := os.Stderr

	// Stream will initiate the transport of the standard shell streams and return an error if a problem occurs.
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})

	if err != nil {
		return "", err
	}

	return out.String(), nil
}

//SetExecCommandAttributes initialise all the pod details  to run exec command
func SetExecCommandAttributes(podDetails *PodDetails, PodName, ContainerName, Namespace string) {

	podDetails.ContainerName = ContainerName
	podDetails.Namespace = Namespace
	podDetails.PodName = PodName
}

// checkPodStatus verify the status of given pod & container
func checkPodStatus(pod *apiv1.Pod, containerName string) error {

	if strings.ToLower(string(pod.Status.Phase)) != "running" {
		return errors.Errorf("%v pod is not in running state, phase: %v", pod.Name, pod.Status.Phase)
	}
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == containerName && !container.Ready {
			return errors.Errorf("%v container of %v pod is not in ready state, phase: %v", container.Name, pod.Name, pod.Status.Phase)
		}
	}
	return nil
}
