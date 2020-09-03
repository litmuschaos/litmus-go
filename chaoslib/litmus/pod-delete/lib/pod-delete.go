package lib

import (
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//PreparePodDelete contains the prepration steps before chaos injection
func PreparePodDelete(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Getting the iteration count for the pod deletion
	GetIterations(experimentsDetails)

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	err = PodDeleteChaos(experimentsDetails, clients, eventsDetails, chaosDetails, resultDetails)
	if err != nil {
		return errors.Errorf("Unable to delete the application pods, due to %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//GetIterations derive the iterations value from given parameters
func GetIterations(experimentsDetails *experimentTypes.ExperimentDetails) {
	var Iterations int
	if experimentsDetails.ChaosInterval != 0 {
		Iterations = experimentsDetails.ChaosDuration / experimentsDetails.ChaosInterval
	} else {
		Iterations = 0
	}
	experimentsDetails.Iterations = math.Maximum(Iterations, 1)

}

//PodDeleteChaos deletes the random single/multiple pods
func PodDeleteChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	GracePeriod := int64(0)
	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

loop:
	for count := 0; count < experimentsDetails.Iterations; count++ {

		// Get the target pod details for the chaos execution
		// if the target pod is not defined it will derive the random target pod list using pod affected percentage
		targetPodList, err := common.GetPodList(experimentsDetails.AppNS, experimentsDetails.TargetPod, experimentsDetails.AppLabel, experimentsDetails.PodsAffectedPerc, clients)
		if err != nil {
			return errors.Errorf("Unable to get the target pod list due to, err: %v", err)
		}

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		//Deleting the application pod
		for _, pod := range targetPodList.Items {

			log.InfoWithValues("[Info]: Killing the following pods", logrus.Fields{
				"PodName": pod.Name})

			if experimentsDetails.Force == true {
				err = clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Delete(pod.Name, &v1.DeleteOptions{GracePeriodSeconds: &GracePeriod})
			} else {
				err = clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Delete(pod.Name, &v1.DeleteOptions{})
			}
			if err != nil {
				return err
			}
		}

		//Waiting for the chaos interval after chaos injection
		if experimentsDetails.ChaosInterval != 0 {
			log.Infof("[Wait]: Wait for the chaos interval %vs", strconv.Itoa(experimentsDetails.ChaosInterval))
			common.WaitForDuration(experimentsDetails.ChaosInterval)
		}

		//Verify the status of pod after the chaos injection
		log.Info("[Status]: Verification for the recreation of application pod")
		if err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			return err
		}

		endTime = time.After(timeDelay)
		select {
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			break loop
		}

	}
	log.Infof("[Completion]: %v chaos is done", experimentsDetails.ExperimentName)

	return nil
}
