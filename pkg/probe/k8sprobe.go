package probe

import (
	"fmt"
	"time"

	"github.com/kyokomi/emoji"
	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// PrepareK8sProbe contains the steps to prepare the k8s probe
// k8s probe can be used to add the probe which needs client-go for command execution, no extra binaries/command
func PrepareK8sProbe(k8sProbes []v1alpha1.K8sProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, phase string, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	if k8sProbes != nil {

		for _, probe := range k8sProbes {

			//DISPLAY THE K8S PROBE INFO
			log.InfoWithValues("[Probe]: The k8s probe informations are as follows", logrus.Fields{
				"Name":            probe.Name,
				"Command":         probe.Inputs.Command,
				"Expected Result": probe.Inputs.ExpectedResult,
				"Run Properties":  probe.RunProperties,
			})

			// triggering the k8s probe and storing the output into the out buffer
			err = TriggerK8sProbe(probe, probe.Inputs.Command, clients)
			// failing the probe, if the success condition doesn't met after the retry & timeout combinations
			if err != nil {
				SetProbeVerdictAfterFailure(resultDetails)
				log.Infof("[Probe]: %v probe has been Failed %v", probe.Name, emoji.Sprint(":cry:"))
				return err
			}
			// counting the passed probes count to generate the score and mark the verdict as passed
			resultDetails.ProbeCount++
			SetProbeVerdict(resultDetails, "Passed", probe.Name, "K8sProbe", "edge", phase)
			log.Infof("[Probe]: The %v probe has been Passed %v", probe.Name, emoji.Sprint(":smile:"))
			resultDetails.PassedProbe = append(resultDetails.PassedProbe, probe.Name+"-"+phase)
		}
	}
	return nil
}

// TriggerK8sProbe run the k8s probe command and storing the output into the out buffer
func TriggerK8sProbe(probe v1alpha1.K8sProbeAttributes, cmd v1alpha1.K8sCommand, clients clients.ClientSets) error {

	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	err = retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {

			//defining the gvr for the requested resource
			gvr := schema.GroupVersionResource{
				Group:    cmd.Group,
				Version:  cmd.Version,
				Resource: cmd.Resource,
			}

			// using dynamic client to get the resource
			resourceList, err := clients.DynamicClient.Resource(gvr).Namespace(cmd.Namespace).List(v1.ListOptions{FieldSelector: cmd.FieldSelector})
			if err != nil || len(resourceList.Items) == 0 {
				return fmt.Errorf("unable to list the resources with matching selector, err: %v", err)
			}
			return nil
		})
	return err
}
