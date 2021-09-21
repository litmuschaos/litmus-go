package vmware

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

//StartVM starts a given powered-off VM
func StartVM(vcenterServer, vmId, cookie string) error {

	req, err := http.NewRequest("POST", "https://"+vcenterServer+"/rest/vcenter/vm/"+vmId+"/power/start", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		var errorResponse ErrorResponse
		json.Unmarshal(body, &errorResponse)
		return errors.Errorf("failed to start vm: %s", errorResponse.MsgValue.MsgMessages[0].MsgDefaultMessage)
	}

	return nil
}

//StopVM stops a given powered-on VM
func StopVM(vcenterServer, vmId, cookie string) error {

	req, err := http.NewRequest("POST", "https://"+vcenterServer+"/rest/vcenter/vm/"+vmId+"/power/stop", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		var errorResponse ErrorResponse
		json.Unmarshal(body, &errorResponse)
		return errors.Errorf("failed to stop vm: %s", errorResponse.MsgValue.MsgMessages[0].MsgDefaultMessage)
	}

	return nil
}
