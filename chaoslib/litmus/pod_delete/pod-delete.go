package pod_delete

import (
	"strconv"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/litmuschaos/litmus-go/pkg/environment"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//GracePeriod set to zero
var GracePeriod int64 = 0
var err error

//PodDeleteChaos deletes the random single/multiple pods
func PodDeleteChaos(experimentsDetails *types.ExperimentDetails, clients environment.ClientSets, Iterations int, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now().Unix()

	for x := 0; x < Iterations; x++ {
		//Getting the list of all the target pod for deletion
		targetPodList, err := PreparePodList(experimentsDetails, clients, resultDetails)
		if err != nil {
			return err
		}
		log.InfoWithValues("[Info]: Killing the following pods", logrus.Fields{
			"PodList": targetPodList})

		//Deleting the application pod
		for _, pods := range targetPodList {
			if experimentsDetails.Force == true {
				err = clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Delete(pods, &v1.DeleteOptions{GracePeriodSeconds: &GracePeriod})
			} else {
				err = clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Delete(pods, &v1.DeleteOptions{})
			}
			if err != nil {
				resultDetails.FailStep = "Deleting the application pod"
				return err
			}
		}
		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
			environment.SetEventAttributes(eventsDetails, types.ChaosInject, msg)
			events.GenerateEvents(experimentsDetails, clients, eventsDetails)
		}

		//ChaosCurrentTimeStamp contains the current timestamp
		ChaosCurrentTimeStamp := time.Now().Unix()

		//ChaosDiffTimeStamp contains the difference of current timestamp and start timestamp
		//It will helpful to track the total chaos duration
		chaosDiffTimeStamp := ChaosCurrentTimeStamp - ChaosStartTimeStamp

		if int(chaosDiffTimeStamp) >= experimentsDetails.ChaosDuration {
			break
		}

		//Waiting for the chaos interval after chaos injection
		if experimentsDetails.ChaosInterval != 0 {
			log.Infof("[Wait]: Wait for the chaos interval %vs", strconv.Itoa(experimentsDetails.ChaosInterval))
			waitForChaosInterval(experimentsDetails)
		}
		//Verify the status of pod after the chaos injection
		log.Info("[Status]: Verification for the recreation of application pod")
		err = status.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
		if err != nil {
			resultDetails.FailStep = "Verification for the recreation of application pod"
			return err
		}
	}
	log.Infof("[Completion]: %v chaos is done", experimentsDetails.ExperimentName)

	return nil
}

//PreparePodDelete contains the steps for prepration before chaos
func PreparePodDelete(experimentsDetails *types.ExperimentDetails, clients environment.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	//It will get the total iteration for the pod-delete
	Iterations := GetIterations(experimentsDetails)
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}
	//Deleting for the application pod
	err := PodDeleteChaos(experimentsDetails, clients, Iterations, resultDetails, eventsDetails)
	if err != nil {
		return err
	}
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}
	return nil
}

//GetIterations derive the iterations value from given parameters
func GetIterations(experimentsDetails *types.ExperimentDetails) int {

	Iterations := experimentsDetails.ChaosDuration / experimentsDetails.ChaosInterval
	return math.Maximum(Iterations, 1)

}

//waitForRampTime waits for the given ramp time duration (in seconds)
func waitForRampTime(experimentsDetails *types.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.RampTime) * time.Second)
}

//waitForChaosInterval waits for the given ramp time duration (in seconds)
func waitForChaosInterval(experimentsDetails *types.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)
}

//PreparePodList derive the list of target pod for deletion
//It is based on the KillCount value
func PreparePodList(experimentsDetails *types.ExperimentDetails, clients environment.ClientSets, resultDetails *types.ResultDetails) ([]string, error) {

	var targetPodList []string

	err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			pods, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
			if err != nil || len(pods.Items) == 0 {
				return errors.Errorf("Unable to get the pod, err: %v", err)
			}
			//Adding the first pod only, if KillCount is not set or 0
			//Otherwise derive the min(KIllCount,len(pod_list)) pod
			if experimentsDetails.KillCount == 0 {
				targetPodList = append(targetPodList, pods.Items[0].Name)
			} else {
				for i := 0; i < math.Minimum(experimentsDetails.KillCount, len(pods.Items)); i++ {
					targetPodList = append(targetPodList, pods.Items[i].Name)
				}
			}

			return nil
		})

	return targetPodList, err
}
