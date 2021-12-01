package redfish

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"

	"fmt"
	"net/http"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

//State helps get the power state of the node
type State struct {
	PowerState string
}

//GetNodeStatus will check and return the status of the node.
func GetNodeStatus(IP, user, password string) (string, error) {
	URL := fmt.Sprintf("https://%v/redfish/v1/Systems/System.Embedded.1/", IP)
	auth := user + ":" + password
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	data := map[string]string{}
	json_data, _ := json.Marshal(data)
	req, err := http.NewRequest("GET", URL, bytes.NewBuffer(json_data))
	if err != nil {
		msg := fmt.Sprintf("Error creating http request: %v", err)
		log.Error(msg)
		return "", errors.Errorf("fail to get the node status, err: %v", err)
	}
	req.Header.Add("Authorization", "Basic "+encodedAuth)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "*/*")
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("Error creating post request: %v", err)
		log.Error(msg)
	}
	log.Infof(resp.Status)
	if resp.StatusCode != 200 {
		log.Error("Unable to get current state of the node")
		return "", errors.Errorf("fail to get the node status. Request failed with status: %v", resp.StatusCode)
	}
	defer resp.Body.Close()
	power := new(State)
	json.NewDecoder(resp.Body).Decode(power)
	return power.PowerState, nil
}

//RebootNode triggers hard reset on the target baremetal node
func RebootNode(URL, user, password string) error {
	data := map[string]string{"ResetType": "ForceRestart"}
	json_data, err := json.Marshal(data)
	auth := user + ":" + password
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	if err != nil {
		log.Error(err.Error())
		return errors.New("Unable to encode the authentication credentials")
	}
	req, err := http.NewRequest("POST", URL, bytes.NewBuffer(json_data))
	if err != nil {
		msg := fmt.Sprintf("Error creating http request: %v", err)
		log.Error(msg)
		return errors.New(msg)
	}
	req.Header.Add("Authorization", "Basic "+encodedAuth)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "*/*")
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("Error creating post request: %v", err)
		log.Error(msg)
		return errors.New(msg)
	}
	log.Infof(resp.Status)
	if resp.StatusCode >= 400 && resp.StatusCode < 200 {
		return errors.New("Failed to trigger node restart")
	}
	defer resp.Body.Close()
	return nil
}
