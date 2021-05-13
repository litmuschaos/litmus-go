package vmware

import (
	"crypto/tls"
	"encoding/json"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/types"
	"io/ioutil"
	"net/http"
)

//GetVMStatus gets the current status of Vcenter VM
func GetVMStatus(experimentsDetails *experimentTypes.ExperimentDetails, cookie string) (string, error) {

	type Message struct {
		MsgValue struct {
			StateValue string `json:"state"`
		} `json:"value"`
	}

	//Leverage Go's HTTP Post function to make request
	req, err := http.NewRequest("GET", "https://"+experimentsDetails.VcenterServer+"/rest/vcenter/vm/"+experimentsDetails.AppVmMoid+"/power/", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	//Handle Error
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	//Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var m1 Message
	json.Unmarshal([]byte(body), &m1)
	return string(m1.MsgValue.StateValue), nil

}
