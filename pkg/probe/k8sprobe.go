package probe

import (
	"fmt"
	"time"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
)

// PrepareK8sProbe contains the steps to prepare the k8s probe
// k8s probe can be used to add the probe which needs client-go for command execution, no extra binaries/command
func PrepareK8sProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, phase string, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	if EligibleForPrint(probe.Mode, phase) {
		//DISPLAY THE K8S PROBE INFO
		log.InfoWithValues("[Probe]: The k8s probe information is as follows", logrus.Fields{
			"Name":            probe.Name,
			"Command":         probe.K8sProbeInputs.Command,
			"Expected Result": probe.K8sProbeInputs.ExpectedResult,
			"Run Properties":  probe.RunProperties,
			"Mode":            probe.Mode,
			"Phase":           phase,
		})
	}

	// triggering probes on the basis of mode & phase so that probe will only run when they are requested to run
	// if mode is SOT & phase is PreChaos, it will trigger Probes in PreChaos section
	// if mode is EOT & phase is PostChaos, it will trigger Probes in PostChaos section
	// if mode is Edge then independent of phase, it will trigger Probes in both Pre/Post Chaos section
	if (probe.Mode == "SOT" && phase == "PreChaos") || (probe.Mode == "EOT" && phase == "PostChaos") || probe.Mode == "Edge" {

		// triggering the k8s probe
		err = TriggerK8sProbe(probe, clients, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, phase); err != nil {
			return err
		}
	}
	// trigger probes for the continuous mode
	if probe.Mode == "Continuous" && phase == "PreChaos" {
		go TriggerContinuousK8sProbe(probe, clients, resultDetails)
	}
	// verify the continuous mode and marked the result of the probes
	if probe.Mode == "Continuous" && phase == "PostChaos" {
		// it will check for the error, It will detect the error if any error encountered in probe during chaos
		err = CheckForErrorInContinuousProbe(resultDetails, probe.Name)
		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		if err = MarkedVerdictInEnd(err, resultDetails, probe.Name, probe.Mode, probe.Type, phase); err != nil {
			return err
		}
	}

	return nil
}

// TriggerK8sProbe run the k8s probe command
func TriggerK8sProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, resultDetails *types.ResultDetails) error {

	cmd := probe.K8sProbeInputs.Command

	// It parse the templated command and return normal string
	// if command doesn't have template, it will return the same command
	cmd.FieldSelector, err = ParseCommand(cmd.FieldSelector, resultDetails)
	if err != nil {
		return err
	}

	cmd.LabelSelector, err = ParseCommand(cmd.LabelSelector, resultDetails)
	if err != nil {
		return err
	}

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

			switch probe.Operation {
			case "create", "Create":
				if err = CreateResource(probe, gvr, clients); err != nil {
					return err
				}
			case "delete", "Delete":
				if err = DeleteResource(probe, gvr, clients); err != nil {
					return err
				}
			case "present", "Present":
				resourceList, err := clients.DynamicClient.Resource(gvr).Namespace(cmd.Namespace).List(v1.ListOptions{
					FieldSelector: cmd.FieldSelector,
					LabelSelector: cmd.LabelSelector,
				})
				if err != nil || len(resourceList.Items) == 0 {
					return fmt.Errorf("unable to list the resources with matching selector, err: %v", err)
				}
			case "absent", "Absent":
				resourceList, err := clients.DynamicClient.Resource(gvr).Namespace(cmd.Namespace).List(v1.ListOptions{
					FieldSelector: cmd.FieldSelector,
					LabelSelector: cmd.LabelSelector,
				})
				if err != nil {
					return fmt.Errorf("unable to list the resources with matching selector, err: %v", err)
				}
				if len(resourceList.Items) != 0 {
					return fmt.Errorf("Resource is not deleted yet due to, err: %v", err)
				}
			default:
				return fmt.Errorf("operation type '%s' not supported in the k8s probe", probe.Operation)
			}

			return nil
		})
	return err
}

// TriggerContinuousK8sProbe trigger the continuous k8s probes
func TriggerContinuousK8sProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails) {
	// it trigger the k8s probe for the entire duration of chaos and it fails, if any error encounter
	// marked the error for the probes, if any
	for {
		err = TriggerK8sProbe(probe, clients, chaosresult)
		// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
		if err != nil {
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
					break
				}

			}
			break
		}

		// waiting for the probe polling interval
		time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
	}
}

// CreateResource creates the resource from the data provided inside data field
func CreateResource(probe v1alpha1.ProbeAttributes, gvr schema.GroupVersionResource, clients clients.ClientSets) error {
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	// Decode YAML manifest into unstructured.Unstructured
	data := &unstructured.Unstructured{}
	_, _, err = decUnstructured.Decode([]byte(probe.Data), nil, data)
	if err != nil {
		return err
	}

	_, err := clients.DynamicClient.Resource(gvr).Namespace(probe.K8sProbeInputs.Command.Namespace).Create(data, v1.CreateOptions{})

	return err
}

// DeleteResource deletes the resource with matching label & field selector
func DeleteResource(probe v1alpha1.ProbeAttributes, gvr schema.GroupVersionResource, clients clients.ClientSets) error {
	resourceList, err := clients.DynamicClient.Resource(gvr).Namespace(probe.K8sProbeInputs.Command.Namespace).List(v1.ListOptions{
		FieldSelector: probe.K8sProbeInputs.Command.FieldSelector,
		LabelSelector: probe.K8sProbeInputs.Command.LabelSelector,
	})
	if err != nil || len(resourceList.Items) == 0 {
		return fmt.Errorf("unable to list the resources with matching selector, err: %v", err)
	}

	for index := range resourceList.Items {
		if err = clients.DynamicClient.Resource(gvr).Namespace(probe.K8sProbeInputs.Command.Namespace).Delete(resourceList.Items[index].GetName(), &v1.DeleteOptions{}); err != nil {
			return err
		}

	}
	return nil
}
