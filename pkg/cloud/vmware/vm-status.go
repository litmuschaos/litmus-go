package vmware

import (
	"net/http"	
	"io/ioutil"	
	"crypto/tls"
	"github.com/pkg/errors"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/types"
)


func GetVMStatus(experimentsDetails *experimentTypes.ExperimentDetails, cookie string) (string, error) {

	//Leverage Go's HTTP Post function to make request
	req, err := http.NewRequest("GET","https://"+ experimentsDetails.VcenterServer +"/rest/vcenter/vm/"+ experimentsDetails.AppVmMoid +"/power/", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
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
	return string(body),nil
}