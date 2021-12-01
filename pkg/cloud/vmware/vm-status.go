package vmware

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

//GetVMStatus returns the current status of a given VM
func GetVMStatus(vcenterServer, vmId, cookie string) (string, error) {

	type VMStatus struct {
		MsgValue struct {
			MsgState string `json:"state"`
		} `json:"value"`
	}

	req, err := http.NewRequest("GET", "https://"+vcenterServer+"/rest/vcenter/vm/"+vmId+"/power/", nil)
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

//VMStatusCheck validates the steady state for the given vm ids
func VMStatusCheck(vcenterServer, vmIds, cookie string) error {

	vmIdList := strings.Split(vmIds, ",")
	if len(vmIdList) == 0 {
		return errors.Errorf("no vm received, please input the target VMMoids")
	}

	for _, vmId := range vmIdList {

		vmStatus, err := GetVMStatus(vcenterServer, vmId, cookie)
		if err != nil {
			return errors.Errorf("failed to get status of %s vm: %s", vmId, err.Error())
		}

		if vmStatus != "POWERED_ON" {
			return errors.Errorf("%s vm is not powered-on", vmId)
		}
	}

	return nil
}
