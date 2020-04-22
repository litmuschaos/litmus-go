package litmus

import (
	"k8s.io/klog"
	"time"

	utils "github.com/litmuschaos/litmus-go/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PodDeleteChaos deletes the random single/multiple pods
func PodDeleteChaos(experimentsDetails *utils.ExperimentDetails, clients utils.ClientSets, Iterations int, failStep *string) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now().Unix()

	for x := 0; x < Iterations; x++ {

		//Getting the list of all the target pod for deletion
		targetPodList, err := PreparePodList(experimentsDetails, clients,failStep)
		if err != nil {
			return err
		}
		klog.V(0).Infof("Killing %v pods", targetPodList)

		//Deleting the application pod
		for _, pods := range targetPodList {
			err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Delete(pods, &metav1.DeleteOptions{})
			if err != nil {
				*failStep = "Deleting the application pod"
				return err
			}
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
			klog.V(0).Infof("[Wait]: Wait for the chaos interval %vs", experimentsDetails.ChaosInterval)
			waitForChaosInterval(experimentsDetails)
		}
		//Verify the status of pod after the chaos injection
		klog.V(0).Infof("[Status]: Verification for the recreation of application pod")
		err = utils.CheckApplicationStatus(experimentsDetails.AppNS, experimentsDetails.AppLabel, clients)
		if err != nil {
			*failStep = "Verification for the recreation of application pod"
			return err
		}
	}

	return nil
}

//PreparePodDelete contains the steps for prepration before chaos
func PreparePodDelete(experimentsDetails *utils.ExperimentDetails, clients utils.ClientSets, failStep *string) error {

	//It will get the total iteration for the pod-delete
	Iterations := GetIterations(experimentsDetails)
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		klog.V(0).Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		waitForRampTime(experimentsDetails)
	}
	//Deleting for the application pod
	err := PodDeleteChaos(experimentsDetails, clients, Iterations, failStep)
	if err != nil{
		return err
	}
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		klog.V(0).Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		waitForRampTime(experimentsDetails)
	}
	return nil
}

//GetIterations derive the iterations value from given parameters
func GetIterations(experimentsDetails *utils.ExperimentDetails) int {

	Iterations := experimentsDetails.ChaosDuration / experimentsDetails.ChaosInterval
	return Maximum(Iterations, 1)

}

//waitForRampTime waits for the given ramp time duration (in seconds)
func waitForRampTime(experimentsDetails *utils.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.RampTime) * time.Second)
}

//waitForChaosInterval waits for the given ramp time duration (in seconds)
func waitForChaosInterval(experimentsDetails *utils.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)
}

// Maximum calculates the maximum value among two integers
func Maximum(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

//Minimum calculates the minimum value among two integers
func Minimum(a int, b int) int {
	if a > b {
		return b
	}
	return a
}

//PreparePodList derive the list of target pod for deletion
//It is based on the KillCount value
func PreparePodList(experimentsDetails *utils.ExperimentDetails, clients utils.ClientSets, failStep *string) ([]string, error) {

	var targetPodList []string
	//Getting the list of pods with the given labels and namespaces
	pods, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(metav1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil {
		*failStep = "Getting the list of pods with the given labels and namespaces"
		return targetPodList, err
	}

	//Adding the first pod only, if KillCount is not set or 0
	//Otherwise derive the min(KIllCount,len(pod_list)) pod
	if experimentsDetails.KillCount == 0 {
		targetPodList = append(targetPodList, pods.Items[0].Name)
	} else {
		for i := 0; i < Minimum(experimentsDetails.KillCount, len(pods.Items)); i++ {
			targetPodList = append(targetPodList, pods.Items[i].Name)
		}
	}
	return targetPodList, err
}
