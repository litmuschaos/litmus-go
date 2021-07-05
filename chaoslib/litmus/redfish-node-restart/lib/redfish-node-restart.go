package lib

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/baremetal/redfish-node-restart/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
)

func injectChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	URL := fmt.Sprintf("https://%v/redfish/v1/Systems/System.Embedded.1/Actions/ComputerSystem.Reset", experimentsDetails.IPMIIP)
	user := experimentsDetails.User
	password := experimentsDetails.Password
	rebootNode(URL, user, password)
	return nil
}

//experimentExecution function orchestrates the experiment by calling the injectChaos function
func experimentExecution(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	return runChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails)
}

func rebootNode(URL, user, password string) {
	data := map[string]string{"ResetType": "ForceRestart"}
	json_data, err := json.Marshal(data)
	auth := user + ":" + password
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	if err != nil {
		log.Fatal(err.Error())
	}
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	req, err := http.NewRequest("POST", URL, bytes.NewBuffer(json_data))
	if err != nil {
		msg := fmt.Sprintf("Error creating http request: %v", err)
		log.Error(msg)
		return
	}
	req.Header.Add("Authorization", "Basic "+encodedAuth)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "*/*")
	client := &http.Client{}
	log.Infof(URL)
	resp, err := client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("Error creating post request: %v", err)
		log.Error(msg)
	}
	log.Infof(resp.Status)
	defer resp.Body.Close()
}

func runChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	// var endTime <-chan time.Time
	// timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.IPMIIP + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	go injectChaos(experimentsDetails, clients)

	log.Infof("[Chaos]:Waiting for: %vs", experimentsDetails.ChaosDuration)

	// signChan channel is used to transmit signal notifications.
	signChan := make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to signChan channel.
	signal.Notify(signChan, os.Interrupt, syscall.SIGTERM)
	// loop:
	// 	for {
	// 		endTime = time.After(timeDelay)
	// 		select {
	// 		case <-signChan:
	// 			log.Info("[Chaos]: Revert Started")
	// 			if err := killChaos(experimentsDetails, pod.Name, clients); err != nil {
	// 				log.Error("unable to kill chaos process after receiving abortion signal")
	// 			}
	// 			log.Info("[Chaos]: Revert Completed")
	// 			os.Exit(1)
	// 		case <-endTime:
	// 			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
	// 			endTime = nil
	// 			break loop
	// 		}
	// 	}
	// 	if err := killChaos(experimentsDetails, clients); err != nil {
	// 		return err
	// 	}

	return nil
}

func PrepareChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	//Starting the CPU stress experiment
	if err := experimentExecution(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
		return err
	}
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// func killChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
// 	// It will contains all the pod & container details required for exec command
// 	execCommandDetails := litmusexec.PodDetails{}

// 	command := []string{"/bin/sh", "-c", experimentsDetails.ChaosKillCmd}

// 	litmusexec.SetExecCommandAttributes(&execCommandDetails, podName, experimentsDetails.TargetContainer, experimentsDetails.AppNS)
// 	_, err := litmusexec.Exec(&execCommandDetails, clients, command)
// 	if err != nil {
// 		return errors.Errorf("unable to kill the process in %v pod, err: %v", podName, err)
// 	}
// 	return nil
// }
