package azure

import (
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/litmuschaos/litmus-go/pkg/log"
)

func GetRunCommandResult(result *compute.RunCommandResult) {
	message := *(*result.Value)[0].Message

	stdout := false
	stderr := false

	for _, line := range strings.Split(strings.TrimSuffix(message, "\n"), "\n") {
		if line == "[stdout]" {
			log.Info("[Info]: Run Command Output: \n")
			stderr = false
			stdout = true
			continue
		}
		if line == "[stderr]" {
			log.Info("[Info]: Run Command Errors: \n")
			stdout = false
			stderr = true
			continue
		}

		if stdout {
			if line != "" {
				log.Infof("[stdout]: %v\n", line)
			}
		}
		if stderr {
			if line != "" {
				log.Errorf("[stderr]: %v\n", line)
			}
		}
	}
}
