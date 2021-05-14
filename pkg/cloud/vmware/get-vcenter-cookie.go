package vmware

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/types"
)

type message struct {
	MsgValue string `json:"value"`
}

//GetVcenterSessionID returns the vcenter sessionid
func GetVcenterSessionID(experimentsDetails *experimentTypes.ExperimentDetails) (string, error) {

	//Leverage Go's HTTP Post function to make request
	req, err := http.NewRequest("POST", "https://"+experimentsDetails.VcenterServer+"/rest/com/vmware/cis/session", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(experimentsDetails.VcenterUser, experimentsDetails.VcenterPass)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	//Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var m message
	json.Unmarshal([]byte(body), &m)

	login := "vmware-api-session-id=" + m.MsgValue + ";Path=/rest;Secure;HttpOnly"
	return login, nil
}
