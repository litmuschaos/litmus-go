package vmware

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// GetVMIDFromTag will fetch the VM IDs from the tag
func GetVMIDFromTag(tag string) (string, error) {

	var vmMoids []string

	command := "govc tags.attached.ls " + tag
	stdout, err := shellout(command)

	if err != nil {
		return "", err
	}
	vmInstances := strings.Split(stdout, "\n")
	for _, vm := range vmInstances {
		if vm != "" {
			vmMoids = append(vmMoids, cleanString(strings.Split(vm, ":")[1]))
		}
	}

	return strings.Join(vmMoids, ","), nil
}

func shellout(command string) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if stderr.String() != "" {
		err = fmt.Errorf(stderr.String())
	}
	return stdout.String(), err
}

func cleanString(s string) string {
	spaceRemoved := strings.Trim(s, " ")
	newLineRemoved := strings.Trim(spaceRemoved, "\n")

	return newLineRemoved
}
