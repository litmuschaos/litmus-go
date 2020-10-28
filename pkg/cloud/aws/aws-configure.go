package aws

import (
	"io"
	"os"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

//ConfigureAWS will setup configuration for AWS
func ConfigureAWS() error {

	var err error
	log.Info("[Authentication]: Creates directory for aws configuration")
	err = os.Mkdir("/root/.aws", 0755)
	if err != nil {
		return err
	}

	log.Info("[Authentication]: Creating credential file in aws directory")
	err = CreateFile("/root/.aws/credentials")
	if err != nil {
		return errors.Errorf("fail to create the credential file err: %v", err)
	}

	log.Info("[Authentication]: Copying aws credentials from cloud_config secret")
	err = Copy("/tmp/cloud_config.yml", "/root/.aws/credentials")
	if err != nil {
		return errors.Errorf("failed to copy credentials err: %v", err)
	}

	return nil
}

//CreateFile creats a new file
func CreateFile(path string) error {

	emptyFile, err := os.Create(path)
	if err != nil {
		return err
	}
	emptyFile.Close()

	return nil
}

//Copy will copy a file from src to dst
func Copy(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return errors.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	return err
}
