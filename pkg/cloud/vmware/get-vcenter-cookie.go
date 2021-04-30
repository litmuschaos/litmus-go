package vmware

import (
	"encoding/json"
	"net/http"	
	"io/ioutil"	
	"crypto/tls"
	
	"github.com/pkg/errors"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/types"
)

type Message struct {
	MsgValue string `json:"value"`
}

//GetVcenterSessionID returns the vcenter sessionid
func GetVcenterSessionID(experimentsDetails *experimentTypes.ExperimentDetails) (string, error) {

	//Leverage Go's HTTP Post function to make request
	req, err := http.NewRequest("POST","https://"+ experimentsDetails.VcenterServer +"/rest/com/vmware/cis/session", nil)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(experimentsDetails.VcenterUser, experimentsDetails.VcenterPass)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	//Handle Error
	if err != nil {
		return "",errors.Errorf("%v", err)
	}
	defer resp.Body.Close()
	//Read the response body
	body, err := ioutil.ReadAll(resp.Body)   
	var m Message
	json.Unmarshal([]byte(body), &m)
   
   
	login := "vmware-api-session-id=" + m.MsgValue + ";Path=/rest;Secure;HttpOnly"
	return login,nil
}
