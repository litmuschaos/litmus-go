package vmware

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
)

// GetVMStatus returns the current status of a given VM
func GetVMStatus(vcenterServer, vmId, cookie string) (string, error) {

	type VMStatus struct {
		MsgValue struct {
			MsgState string `json:"state"`
		} `json:"value"`
	}

	req, err := http.NewRequest("GET", "https://"+vcenterServer+"/rest/vcenter/vm/"+vmId+"/power/", nil)
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason: fmt.Sprintf("failed to get VM status: %v", err.Error()),
			Target: fmt.Sprintf("{VM ID: %v}", vmId),
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
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("failed to get VM status: %v", err.Error()),
			Target:    fmt.Sprintf("{VM ID: %v}", vmId),
		}
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("failed to get VM status: %v", err.Error()),
			Target:    fmt.Sprintf("{VM ID: %v}", vmId),
		}
	}

	if resp.StatusCode != http.StatusOK {

		var errorResponse ErrorResponse
		var reason string

		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			reason = fmt.Sprintf("failed to unmarshal error response: %v", err)
		} else {
			reason = fmt.Sprintf("failed to fetch VM status: %v", errorResponse.MsgValue.MsgMessages[0].MsgDefaultMessage)
		}

		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    reason,
			Target:    fmt.Sprintf("{VM ID: %v}", vmId),
		}
	}

	var vmStatus VMStatus
	if err = json.Unmarshal(body, &vmStatus); err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("failed to unmarshal VM status: %v", err),
			Target:    fmt.Sprintf("{VM ID: %v}", vmId),
		}
	}

	return vmStatus.MsgValue.MsgState, nil
}

// VMStatusCheck validates the steady state for the given vm ids
func VMStatusCheck(vcenterServer, vmIds, cookie string) error {

	vmIdList := strings.Split(vmIds, ",")
	if vmIds == "" || len(vmIdList) == 0 {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    "no VMoid found, please provide target VMMoids",
		}
	}

	for _, vmId := range vmIdList {

		vmStatus, err := GetVMStatus(vcenterServer, vmId, cookie)
		if err != nil {
			return stacktrace.Propagate(err, "failed to get status of VM")
		}

		if vmStatus != "POWERED_ON" {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeStatusChecks,
				Reason:    "VM is not in POWERED_ON state",
				Target:    fmt.Sprintf("{VM ID: %v}", vmId),
			}
		}
	}

	return nil
}
