package vmware

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

type ErrorResponse struct {
	MsgValue struct {
		MsgMessages []struct {
			MsgDefaultMessage string `json:"default_message"`
		} `json:"messages"`
	} `json:"value"`
}

//GetVcenterSessionID returns the vcenter sessionid
func GetVcenterSessionID(vcenterServer, vcenterUser, vcenterPass string) (string, error) {

	type Cookie struct {
		MsgValue string `json:"value"`
	}

	req, err := http.NewRequest("POST", "https://"+vcenterServer+"/rest/com/vmware/cis/session", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(vcenterUser, vcenterPass)
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
		return "", errors.Errorf("error during authentication: %s", errorResponse.MsgValue.MsgMessages[0].MsgDefaultMessage)
	}

	var cookie Cookie
	json.Unmarshal(body, &cookie)

	login := "vmware-api-session-id=" + cookie.MsgValue + ";Path=/rest;Secure;HttpOnly"
	return login, nil
}
