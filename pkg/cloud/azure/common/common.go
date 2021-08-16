package common

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// StringInSlice will check and return whether a string is present inside a slice or not
func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// GetSubscriptionID fetch the subscription id from the auth file and export it in experiment struct variable
func GetSubscriptionID() (string, error) {

	var err error
	authFile, err := os.Open(os.Getenv("AZURE_AUTH_LOCATION"))
	if err != nil {
		return "", errors.Errorf("fail to open auth file, err: %v", err)
	}

	authFileContent, err := ioutil.ReadAll(authFile)
	if err != nil {
		return "", errors.Errorf("fail to read auth file, err: %v", err)
	}

	details := make(map[string]string)
	if err := json.Unmarshal(authFileContent, &details); err != nil {
		return "", errors.Errorf("fail to unmarshal file, err: %v", err)
	}

	if id, contains := details["subscriptionId"]; contains {
		return id, nil
	} else {
		return "", errors.Errorf("The auth file does not have a subscriptionId field")
	}
}

// GetScaleSetNameAndInstanceId extracts the scale set name and VM id from the instance name
func GetScaleSetNameAndInstanceId(instanceName string) (string, string) {
	scaleSetAndInstanceId := strings.Split(instanceName, "_")
	return scaleSetAndInstanceId[0], scaleSetAndInstanceId[1]
}
