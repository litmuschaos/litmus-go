package pod_delete

import (
	"math/rand"
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-delete/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"

	"github.com/openebs/maya/pkg/util/retry"
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
		waitForRampTime(experimentsDetails)
	}
	if err != nil {
		return errors.Errorf("Unable to get the serviceAccountName, err: %v", err)
	}

	err = PodDeleteChaos(experimentsDetails, clients, eventsDetails, chaosDetails)

	if err != nil {
		return errors.Errorf("Unable to delete the application pods, due to %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
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

//waitForRampTime waits for the given ramp time duration (in seconds)
func waitForRampTime(experimentsDetails *experimentTypes.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.RampTime) * time.Second)
}

//PodDeleteChaos deletes the random single/multiple pods
func PodDeleteChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now().Unix()
	var GracePeriod int64 = 0

	for x := 0; x < experimentsDetails.Iterations; x++ {
		//Getting the list of all the target pod for deletion
		targetPodList, err := PreparePodList(experimentsDetails, clients)
		if err != nil {
			return err
		}
		log.InfoWithValues("[Info]: Killing the following pods", logrus.Fields{
			"PodList": targetPodList})

		if experimentsDetails.EngineName != "" {
			types.SetEngineEventAttributes(eventsDetails, types.PreChaosCheck, "Injecting "+experimentsDetails.ExperimentName+" chaos on application pod", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		//Deleting the application pod
		for _, pods := range targetPodList {
			if experimentsDetails.Force == true {
				err = clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Delete(pods, &v1.DeleteOptions{GracePeriodSeconds: &GracePeriod})
			} else {
				err = clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Delete(pods, &v1.DeleteOptions{})
			}
		}
		if err != nil {
			return err
		}

		//Waiting for the chaos interval after chaos injection
		if experimentsDetails.ChaosInterval != 0 {
			log.Infof("[Wait]: Wait for the chaos interval %vs", strconv.Itoa(experimentsDetails.ChaosInterval))
			waitForChaosInterval(experimentsDetails)
		}
		//Verify the status of pod after the chaos injection
		log.Info("[Status]: Verification for the recreation of application pod")
		err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)

		//ChaosCurrentTimeStamp contains the current timestamp
		ChaosCurrentTimeStamp := time.Now().Unix()

		//ChaosDiffTimeStamp contains the difference of current timestamp and start timestamp
		//It will helpful to track the total chaos duration
		chaosDiffTimeStamp := ChaosCurrentTimeStamp - ChaosStartTimeStamp

		if int(chaosDiffTimeStamp) >= experimentsDetails.ChaosDuration {
			break
		}

	}
	log.Infof("[Completion]: %v chaos is done", experimentsDetails.ExperimentName)

	return nil
}

//waitForChaosInterval waits for the given ramp time duration (in seconds)
func waitForChaosInterval(experimentsDetails *experimentTypes.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)
}

//PreparePodList derive the list of target pod for deletion
//It is based on the KillCount value
func PreparePodList(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) ([]string, error) {

	var targetPodList []string

	err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			pods, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
			if err != nil || len(pods.Items) == 0 {
				return errors.Errorf("Unable to get the pod, err: %v", err)
			}
			index := rand.Intn(len(pods.Items))
			//Adding the first pod only, if KillCount is not set or 0
			//Otherwise derive the min(KIllCount,len(pod_list)) pod
			if experimentsDetails.KillCount == 0 {
				targetPodList = append(targetPodList, pods.Items[index].Name)
			} else {
				for i := 0; i < math.Minimum(experimentsDetails.KillCount, len(pods.Items)); i++ {
					targetPodList = append(targetPodList, pods.Items[index].Name)
					index = (index + 1) % len(pods.Items)
				}
			}

			return nil
		})

	return targetPodList, err
}
