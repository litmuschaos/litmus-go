package vmware

import (
	"crypto/tls"
	"net/http"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/types"
)

//StopVM stops the desired VMWare VM
func StopVM(experimentsDetails *experimentTypes.ExperimentDetails, cookie string) error {

	//Leverage Go's HTTP Post function to make request
	req, err := http.NewRequest("POST", "https://"+experimentsDetails.VcenterServer+"/rest/vcenter/vm/"+experimentsDetails.AppVMMoid+"/power/stop", nil)
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
	//Handle Error
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
