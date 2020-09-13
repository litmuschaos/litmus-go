package ip

import "testing"

func TestIpLinkList(t *testing.T) {
	json := `
[
	{
		"ifindex":1,
		"ifname":"lo",
		"flags":[
			"LOOPBACK",
			"UP",
			"LOWER_UP"
		],
		"mtu":65536,
		"qdisc":"noqueue",
		"operstate":"UNKNOWN",
		"linkmode":"DEFAULT",
		"group":"default",
		"txqlen":1000,
		"link_type":"loopback",
		"address":"00:00:00:00:00:00",
		"broadcast":"00:00:00:00:00:00"
	},
	{
		"ifindex":3,
		"link_index":27,
		"ifname":"eth0",
		"flags": [
			"BROADCAST",
			"MULTICAST",
			"UP",
			"LOWER_UP"
		],
		"mtu":1450,
		"qdisc":"noqueue",
		"operstate":"UP",
		"linkmode":"DEFAULT",
		"group":"default",
		"link_type":"ether",
		"address":"0a:58:0a:80:00:0f",
		"broadcast":"ff:ff:ff:ff:ff:ff",
		"link_netnsid":0
	}
]
`

	links, err := parseLinksResponse([]byte(json))
	if err != nil {
		t.Fatalf("Failed to parse ip link json: %s", err)
	}
	expected := 2
	got := len(links)
	if got != expected {
		t.Errorf("Failed to parse ip link json. Expected %d, got %d: %v", expected, got, links)
	}
}
