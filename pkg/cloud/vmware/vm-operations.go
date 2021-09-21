package vmware

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
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

//WaitForVMStart waits for the given VM to attain the POWERED_ON state
func WaitForVMStart(timeout, delay int, vcenterServer, vmId, cookie string) error {

	log.Infof("[Status]: Checking %s VM status", vmId)
	return retry.Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			vmStatus, err := GetVMStatus(vcenterServer, vmId, cookie)
			if err != nil {
				return errors.Errorf("failed to get %s VM status: %s", vmId, err.Error())
			}

			if vmStatus != "POWERED_ON" {
				log.Infof("%s VM state is %s", vmId, vmStatus)
				return errors.Errorf("%s vm is not yet in POWERED_ON state", vmId)
			}

			log.Infof("%s VM state is %s", vmId, vmStatus)
			return nil
		})
}

//WaitForVMStop waits for the given VM to attain the POWERED_OFF state
func WaitForVMStop(timeout, delay int, vcenterServer, vmId, cookie string) error {

	log.Infof("[Status]: Checking %s VM status", vmId)
	return retry.Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			vmStatus, err := GetVMStatus(vcenterServer, vmId, cookie)
			if err != nil {
				return errors.Errorf("failed to get %s VM status: %s", vmId, err.Error())
			}

			if vmStatus != "POWERED_OFF" {
				log.Infof("%s VM state is %s", vmId, vmStatus)
				return errors.Errorf("%s vm is not yet in POWERED_OFF state", vmId)
			}

			log.Infof("%s VM state is %s", vmId, vmStatus)
			return nil
		})
}
