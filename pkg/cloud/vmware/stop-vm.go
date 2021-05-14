package vmware

import (
	"crypto/tls"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/vmware/vm-poweroff/types"
	"net/http"
)

//StopVM stops the desired VMWare VM
func StopVM(experimentsDetails *experimentTypes.ExperimentDetails, cookie string) error {

	//Leverage Go's HTTP Post function to make request
	req, err := http.NewRequest("POST", "https://"+experimentsDetails.VcenterServer+"/rest/vcenter/vm/"+experimentsDetails.AppVmMoid+"/power/stop", nil)
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
