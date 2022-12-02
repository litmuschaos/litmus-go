package vmware

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
)

// ErrorResponse contains error response code
type ErrorResponse struct {
	MsgValue struct {
		MsgMessages []struct {
			MsgDefaultMessage string `json:"default_message"`
		} `json:"messages"`
	} `json:"value"`
}

// GetVcenterSessionID returns the vcenter sessionid
func GetVcenterSessionID(vcenterServer, vcenterUser, vcenterPass string) (string, error) {

	type Cookie struct {
		MsgValue string `json:"value"`
	}

	req, err := http.NewRequest("POST", "https://"+vcenterServer+"/rest/com/vmware/cis/session", nil)
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("failed to get vcenter session id: %v", err.Error())}
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(vcenterUser, vcenterPass)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("failed to get vcenter session id: %v", err.Error())}
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("failed to get vcenter session id: %v", err.Error())}
	}

	if resp.StatusCode != http.StatusOK {

		var errorResponse ErrorResponse
		var reason string

		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			reason = fmt.Sprintf("failed to unmarshal error response: %v", err)
		} else {
			reason = fmt.Sprintf("error during authentication: %v", errorResponse.MsgValue.MsgMessages[0].MsgDefaultMessage)
		}

		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeGeneric,
			Reason:    reason,
		}
	}

	var cookie Cookie

	if err = json.Unmarshal(body, &cookie); err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("failed to unmarshal cookie: %v", err),
		}
	}

	login := "vmware-api-session-id=" + cookie.MsgValue + ";Path=/rest;Secure;HttpOnly"
	return login, nil
}
