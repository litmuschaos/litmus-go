package probe

import (
	"strings"
	"time"

	"github.com/litmuschaos/chaos-operator/pkg/apis/litmuschaos/v1alpha1"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
)

// prepareK8sProbe contains the steps to prepare the k8s probe
// k8s probe can be used to add the probe which needs client-go for command execution, no extra binaries/command
func prepareK8sProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, phase string, chaosDetails *types.ChaosDetails) error {
	switch strings.ToLower(phase) {
	case "prechaos":
		if err := preChaosK8sProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "postchaos":
		if err := postChaosK8sProbe(probe, resultDetails, clients, chaosDetails); err != nil {
			return err
		}
	case "duringchaos":
		onChaosK8sProbe(probe, resultDetails, clients, chaosDetails)
	default:
		return errors.Errorf("phase '%s' not supported in the k8s probe", phase)
	}
	return nil
}

// triggerK8sProbe run the k8s probe command
func triggerK8sProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, resultDetails *types.ResultDetails) error {

	inputs := probe.K8sProbeInputs

	// It parse the templated command and return normal string
	// if command doesn't have template, it will return the same command
	inputs.FieldSelector, err = parseCommand(inputs.FieldSelector, resultDetails)
	if err != nil {
		return err
	}

	inputs.LabelSelector, err = parseCommand(inputs.LabelSelector, resultDetails)
	if err != nil {
		return err
	}

	// it will retry for some retry count, in each iterations of try it contains following things
	// it contains a timeout per iteration of retry. if the timeout expires without success then it will go to next try
	// for a timeout, it will run the command, if it fails wait for the iterval and again execute the command until timeout expires
	return retry.Times(uint(probe.RunProperties.Retry)).
		Timeout(int64(probe.RunProperties.ProbeTimeout)).
		Wait(time.Duration(probe.RunProperties.Interval) * time.Second).
		TryWithTimeout(func(attempt uint) error {
			//defining the gvr for the requested resource
			gvr := schema.GroupVersionResource{
				Group:    inputs.Group,
				Version:  inputs.Version,
				Resource: inputs.Resource,
			}

			switch strings.ToLower(inputs.Operation) {
			case "create":
				if err = createResource(probe, gvr, clients); err != nil {
					log.Errorf("the %v k8s probe has Failed, err: %v", probe.Name, err)
					return err
				}
			case "delete":
				if err = deleteResource(probe, gvr, clients); err != nil {
					log.Errorf("the %v k8s probe has Failed, err: %v", probe.Name, err)
					return err
				}
			case "present":
				resourceList, err := clients.DynamicClient.Resource(gvr).Namespace(inputs.Namespace).List(v1.ListOptions{
					FieldSelector: inputs.FieldSelector,
					LabelSelector: inputs.LabelSelector,
				})
				if err != nil {
					log.Errorf("the %v k8s probe has Failed, err: %v", probe.Name, err)
					return errors.Errorf("unable to list the resources with matching selector, err: %v", err)
				} else if len(resourceList.Items) == 0 {
					return errors.Errorf("no resource found with provided selectors")
				}
			case "absent":
				resourceList, err := clients.DynamicClient.Resource(gvr).Namespace(inputs.Namespace).List(v1.ListOptions{
					FieldSelector: inputs.FieldSelector,
					LabelSelector: inputs.LabelSelector,
				})
				if err != nil {
					return errors.Errorf("unable to list the resources with matching selector, err: %v", err)
				}
				if len(resourceList.Items) != 0 {
					log.Errorf("the %v k8s probe has Failed, err: %v", probe.Name, err)
					return errors.Errorf("resource is not deleted yet due to, err: %v", err)
				}
			default:
				return errors.Errorf("operation type '%s' not supported in the k8s probe", inputs.Operation)
			}

			return nil
		})
}

// triggerContinuousK8sProbe trigger the continuous k8s probes
func triggerContinuousK8sProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) {
	var isExperimentFailed bool
	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
	}

	// it trigger the k8s probe for the entire duration of chaos and it fails, if any error encounter
	// marked the error for the probes, if any
loop:
	for {
		err = triggerK8sProbe(probe, clients, chaosresult)
		// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
		if err != nil {
			for index := range chaosresult.ProbeDetails {
				if chaosresult.ProbeDetails[index].Name == probe.Name {
					chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
					log.Errorf("the %v k8s probe has been Failed, err: %v", probe.Name, err)
					isExperimentFailed = true
					break loop
				}
			}
		}
		// waiting for the probe polling interval
		time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
	}
	// if experiment fails and stopOnfailure is provided as true then it will patch the chaosengine for abort
	// if experiment fails but stopOnfailure is provided as false then it will continue the execution
	// and failed the experiment in the end
	if isExperimentFailed && probe.RunProperties.StopOnFailure {
		if err := stopChaosEngine(probe, clients, chaosresult, chaosDetails); err != nil {
			log.Errorf("unable to patch chaosengine to stop, err: %v", err)
		}
	}
}

// createResource creates the resource from the data provided inside data field
func createResource(probe v1alpha1.ProbeAttributes, gvr schema.GroupVersionResource, clients clients.ClientSets) error {
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	// Decode YAML manifest into unstructured.Unstructured
	data := &unstructured.Unstructured{}
	_, _, err = decUnstructured.Decode([]byte(probe.Data), nil, data)
	if err != nil {
		return err
	}
	_, err := clients.DynamicClient.Resource(gvr).Namespace(probe.K8sProbeInputs.Namespace).Create(data, v1.CreateOptions{})

	return err
}

// deleteResource deletes the resource with matching label & field selector
func deleteResource(probe v1alpha1.ProbeAttributes, gvr schema.GroupVersionResource, clients clients.ClientSets) error {
	resourceList, err := clients.DynamicClient.Resource(gvr).Namespace(probe.K8sProbeInputs.Namespace).List(v1.ListOptions{
		FieldSelector: probe.K8sProbeInputs.FieldSelector,
		LabelSelector: probe.K8sProbeInputs.LabelSelector,
	})
	if err != nil {
		return errors.Errorf("unable to list the resources with matching selector, err: %v", err)
	} else if len(resourceList.Items) == 0 {
		return errors.Errorf("no resource found with provided selectors")
	}

	for index := range resourceList.Items {
		if err = clients.DynamicClient.Resource(gvr).Namespace(probe.K8sProbeInputs.Namespace).Delete(resourceList.Items[index].GetName(), &v1.DeleteOptions{}); err != nil {
			return err
		}
	}
	return nil
}

//preChaosK8sProbe trigger the k8s probe for prechaos phase
func preChaosK8sProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch strings.ToLower(probe.Mode) {
	case "sot", "edge":

		//DISPLAY THE K8S PROBE INFO
		log.InfoWithValues("[Probe]: The k8s probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Inputs":         probe.K8sProbeInputs,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PreChaos",
		})
		// waiting for initial delay
		if probe.RunProperties.InitialDelaySeconds != 0 {
			log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
			time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		}
		// triggering the k8s probe
		err = triggerK8sProbe(probe, clients, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = markedVerdictInEnd(err, resultDetails, probe, "PreChaos"); err != nil {
			return err
		}
	case "continuous":

		//DISPLAY THE K8S PROBE INFO
		log.InfoWithValues("[Probe]: The k8s probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Inputs":         probe.K8sProbeInputs,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PreChaos",
		})
		go triggerContinuousK8sProbe(probe, clients, resultDetails, chaosDetails)
	}
	return nil
}

//postChaosK8sProbe trigger the k8s probe for postchaos phase
func postChaosK8sProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	switch strings.ToLower(probe.Mode) {
	case "eot", "edge":

		//DISPLAY THE K8S PROBE INFO
		log.InfoWithValues("[Probe]: The k8s probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Inputs":         probe.K8sProbeInputs,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "PostChaos",
		})
		// waiting for initial delay
		if probe.RunProperties.InitialDelaySeconds != 0 {
			log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
			time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		}
		// triggering the k8s probe
		err = triggerK8sProbe(probe, clients, resultDetails)

		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		// it will update the status of all the unrun probes as well
		if err = markedVerdictInEnd(err, resultDetails, probe, "PostChaos"); err != nil {
			return err
		}
	case "continuous", "onchaos":
		// it will check for the error, It will detect the error if any error encountered in probe during chaos
		err = checkForErrorInContinuousProbe(resultDetails, probe.Name)
		// failing the probe, if the success condition doesn't met after the retry & timeout combinations
		if err = markedVerdictInEnd(err, resultDetails, probe, "PostChaos"); err != nil {
			return err
		}
	}
	return nil
}

//onChaosK8sProbe trigger the k8s probe for DuringChaos phase
func onChaosK8sProbe(probe v1alpha1.ProbeAttributes, resultDetails *types.ResultDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails) {

	switch strings.ToLower(probe.Mode) {
	case "onchaos":

		//DISPLAY THE K8S PROBE INFO
		log.InfoWithValues("[Probe]: The k8s probe information is as follows", logrus.Fields{
			"Name":           probe.Name,
			"Inputs":         probe.K8sProbeInputs,
			"Run Properties": probe.RunProperties,
			"Mode":           probe.Mode,
			"Phase":          "DuringChaos",
		})
		go triggerOnChaosK8sProbe(probe, clients, resultDetails, chaosDetails)
	}

}

// triggerOnChaosK8sProbe trigger the onchaos k8s probes
func triggerOnChaosK8sProbe(probe v1alpha1.ProbeAttributes, clients clients.ClientSets, chaosresult *types.ResultDetails, chaosDetails *types.ChaosDetails) {

	var isExperimentFailed bool
	duration := chaosDetails.ChaosDuration
	// waiting for initial delay
	if probe.RunProperties.InitialDelaySeconds != 0 {
		log.Infof("[Wait]: Waiting for %vs before probe execution", probe.RunProperties.InitialDelaySeconds)
		time.Sleep(time.Duration(probe.RunProperties.InitialDelaySeconds) * time.Second)
		duration = math.Maximum(0, duration-probe.RunProperties.InitialDelaySeconds)
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(duration) * time.Second

	// it trigger the k8s probe for the entire duration of chaos and it fails, if any error encounter
	// marked the error for the probes, if any
loop:
	for {
		endTime = time.After(timeDelay)
		select {
		case <-endTime:
			log.Infof("[Chaos]: Time is up for the %v probe", probe.Name)
			endTime = nil
			break loop
		default:
			err = triggerK8sProbe(probe, clients, chaosresult)
			// record the error inside the probeDetails, we are maintaining a dedicated variable for the err, inside probeDetails
			if err != nil {
				for index := range chaosresult.ProbeDetails {
					if chaosresult.ProbeDetails[index].Name == probe.Name {
						chaosresult.ProbeDetails[index].IsProbeFailedWithError = err
						log.Errorf("The %v k8s probe has been Failed, err: %v", probe.Name, err)
						isExperimentFailed = true
						break loop
					}
				}
			}
			// waiting for the probe polling interval
			time.Sleep(time.Duration(probe.RunProperties.ProbePollingInterval) * time.Second)
		}
	}
	// if experiment fails and stopOnfailure is provided as true then it will patch the chaosengine for abort
	// if experiment fails but stopOnfailure is provided as false then it will continue the execution
	// and failed the experiment in the end
	if isExperimentFailed && probe.RunProperties.StopOnFailure {
		if err := stopChaosEngine(probe, clients, chaosresult, chaosDetails); err != nil {
			log.Errorf("unable to patch chaosengine to stop, err: %v", err)
		}
	}
}
