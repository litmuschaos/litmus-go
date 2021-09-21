package vmware

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

//GetVMStatus returns the current status of a given VM
func GetVMStatus(vcenterServer, appVMMoid, cookie string) (string, error) {

	type VMStatus struct {
		MsgValue struct {
			MsgState string `json:"state"`
		} `json:"value"`
	}

	req, err := http.NewRequest("GET", "https://"+vcenterServer+"/rest/vcenter/vm/"+appVMMoid+"/power/", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {

		var errorResponse ErrorResponse
		json.Unmarshal(body, &errorResponse)
		return "", errors.Errorf("failed to fetch vm status: %s", errorResponse.MsgValue.MsgMessages[0].MsgDefaultMessage)
	}

	var vmStatus VMStatus
	json.Unmarshal(body, &vmStatus)

	return vmStatus.MsgValue.MsgState, nil
}
