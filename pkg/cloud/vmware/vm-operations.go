package vmware

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/palantir/stacktrace"
)

// StartVM starts a given powered-off VM
func StartVM(vcenterServer, vmId, cookie string) error {

	req, err := http.NewRequest("POST", "https://"+vcenterServer+"/rest/vcenter/vm/"+vmId+"/power/start", nil)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    fmt.Sprintf("failed to start VM: %v", err.Error()),
			Target:    fmt.Sprintf("{VM ID: %v}", vmId),
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    fmt.Sprintf("failed to start VM: %v", err.Error()),
			Target:    fmt.Sprintf("{VM ID: %v}", vmId),
		}
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosRevert,
				Reason:    fmt.Sprintf("failed to start VM: %v", err.Error()),
				Target:    fmt.Sprintf("{VM ID: %v}", vmId),
			}
		}

		var errorResponse ErrorResponse
		var reason string

		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			reason = fmt.Sprintf("failed to unmarshal error response: %v", err)
		} else {
			reason = fmt.Sprintf("failed to start VM: %v", errorResponse.MsgValue.MsgMessages[0].MsgDefaultMessage)
		}

		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    reason,
			Target:    fmt.Sprintf("{VM ID: %v}", vmId),
		}
	}

	return nil
}

// StopVM stops a given powered-on VM
func StopVM(vcenterServer, vmId, cookie string) error {

	req, err := http.NewRequest("POST", "https://"+vcenterServer+"/rest/vcenter/vm/"+vmId+"/power/stop", nil)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to stop VM: %v", err.Error()),
			Target:    fmt.Sprintf("{VM ID: %v}", vmId),
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to stop VM: %v", err.Error()),
			Target:    fmt.Sprintf("{VM ID: %v}", vmId),
		}
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosInject,
				Reason:    fmt.Sprintf("failed to stop VM: %v", err.Error()),
				Target:    fmt.Sprintf("{VM ID: %v}", vmId),
			}
		}

		var errorResponse ErrorResponse
		var reason string

		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			reason = fmt.Sprintf("failed to unmarshal error response: %v", err)
		} else {
			reason = fmt.Sprintf("failed to stop VM: %v", errorResponse.MsgValue.MsgMessages[0].MsgDefaultMessage)
		}
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    reason,
			Target:    fmt.Sprintf("{VM ID: %v}", vmId),
		}
	}

	return nil
}

// WaitForVMStart waits for the given VM to attain the POWERED_ON state
func WaitForVMStart(timeout, delay int, vcenterServer, vmId, cookie string) error {

	log.Infof("[Status]: Checking %v VM status", vmId)
	return retry.Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			vmStatus, err := GetVMStatus(vcenterServer, vmId, cookie)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get VM status")
			}

			if vmStatus != "POWERED_ON" {
				log.Infof("%v VM state is %v", vmId, vmStatus)
				return cerrors.Error{
					ErrorCode: cerrors.ErrorTypeChaosRevert,
					Reason:    "VM is not in POWERED_ON state",
					Target:    fmt.Sprintf("{VM ID: %v}", vmId),
				}
			}

			log.Infof("%v VM state is %v", vmId, vmStatus)
			return nil
		})
}

// WaitForVMStop waits for the given VM to attain the POWERED_OFF state
func WaitForVMStop(timeout, delay int, vcenterServer, vmId, cookie string) error {

	log.Infof("[Status]: Checking %v VM status", vmId)
	return retry.Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			vmStatus, err := GetVMStatus(vcenterServer, vmId, cookie)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get VM status")
			}

			if vmStatus != "POWERED_OFF" {
				log.Infof("%v VM state is %v", vmId, vmStatus)
				return cerrors.Error{
					ErrorCode: cerrors.ErrorTypeChaosInject,
					Reason:    "VM is not in POWERED_OFF state",
					Target:    fmt.Sprintf("{VM ID: %v}", vmId),
				}
			}

			log.Infof("%v VM state is %v", vmId, vmStatus)
			return nil
		})
}
