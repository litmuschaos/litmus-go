package aws

import (
	"fmt"
	"os/exec"
	"os/user"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

//ConfigureAWS will setup configuration for AWS
func ConfigureAWS() error {

	var err error
	log.Info("[Authentication]: Creates directory for aws configuration")
	user, err := user.Current()
	if err != nil {
		return errors.Errorf("fail to get the curent user, err: %v", err)
	}
	path := "/" + user.Username + "/.aws"

	out, err := exec.Command("sudo", "mkdir", path).CombinedOutput()
	if err != nil {
		log.Error(fmt.Sprintf("fail to create directory, err: %v", string(out)))
		return err
	}

	log.Info("[Authentication]: Creating credential file in aws directory")
	out, err = exec.Command("sudo", "touch", path+"/credentials").CombinedOutput()
	if err != nil {
		log.Error(fmt.Sprintf("failed to create the credential file, err: %v", string(out)))
		return err
	}

	log.Info("[Authentication]: Copying aws credentials from cloud_config secret")
	out, err = exec.Command("sudo", "cp", "/tmp/cloud_config.yml", path+"/credentials").CombinedOutput()
	if err != nil {
		log.Error(fmt.Sprintf("failed to create the credential file, err: %v", string(out)))
		return err
	}

	return nil
}
