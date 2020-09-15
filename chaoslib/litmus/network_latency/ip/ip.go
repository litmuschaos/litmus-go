package ip

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

// InterfaceName returns the name of the ethernet interface of the given
// process (container). It returns an error if case none, or more than one,
// interface is present.
func InterfaceName(PID int) (string, error) {
	ip := fmt.Sprintf("nsenter -t %d -n ip -json link list", PID)
	cmd := exec.Command("/bin/bash", "-c", ip)
	out, err := cmd.CombinedOutput()
	log.Info(fmt.Sprintf("[ip] %s", cmd))
	if err != nil {
		log.Error(fmt.Sprintf("[ip] Failed to run ip command: %s", string(out)))
		return "", err
	}

	links, err := parseLinksResponse(out)
	if err != nil {
		log.Errorf("[ip] Failed to parse json response from ip command", err)
		return "", err
	}

	ls := []Link{}
	for _, iface := range links {
		if iface.Type != "loopback" {
			ls = append(ls, iface)
		}
	}

	log.Info(fmt.Sprintf("[ip] Found %d link interface(s): %+v", len(ls), ls))
	if len(ls) > 1 {
		errors.Errorf("[ip] Unexpected number of link interfaces for process %d. Expected 1 ethernet link, found %d",
			PID, len(ls))
	}

	return ls[0].Name, nil
}

type LinkListResponse struct {
	Links []Link
}

type Link struct {
	Name  string `json:"ifname"`
	Type  string `json:"link_type"`
	Qdisc string `json:"qdisc"`
	NSID  int    `json:"link_netnsid"`
}

func parseLinksResponse(j []byte) ([]Link, error) {
	var links []Link
	err := json.Unmarshal(j, &links)
	if err != nil {
		return nil, err
	}

	return links, nil
}
