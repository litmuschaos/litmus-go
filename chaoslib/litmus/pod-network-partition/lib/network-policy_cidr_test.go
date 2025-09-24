package lib

import (
	"testing"

	partitionTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-network-partition/types"
)

func Test_SetExceptIPs_CIDRHandling(t *testing.T) {
	np := &NetworkPolicy{}
	exp := &partitionTypes.ExperimentDetails{
		DestinationIPs: "10.0.0.5,10.0.0.0/24,10.0.1.0/28, 2001:db8::1,2001:db8::/64,10.0.0.5,2001:db8::1/64",
	}
	if err := np.setExceptIPs(exp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{
		"10.0.0.5/32",
		"10.0.0.0/24",
		"10.0.1.0/28",
		"2001:db8::1/128",
		"2001:db8::/64",
		"2001:db8::1/64", 
	}
	got := np.ExceptIPs
	if len(got) != len(want) {
		t.Fatalf("len mismatch got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d got=%s want=%s full=%v", i, got[i], want[i], got)
		}
	}
}

func Test_normalizeIPOrCIDR(t *testing.T) {
	cases := map[string]string{
		"10.1.1.1":      "10.1.1.1/32",
		"10.1.1.0/28":   "10.1.1.0/28",
		"2001:db8::1":   "2001:db8::1/128",
		"2001:db8::/64": "2001:db8::/64",
	}
	for in, expect := range cases {
		out, err := normalizeIPOrCIDR(in)
		if err != nil {
			t.Fatalf("unexpected err for %s: %v", in, err)
		}
		if out != expect {
			t.Fatalf("normalize %s got %s want %s", in, out, expect)
		}
	}
	bad := []string{"", "foo", "10.0.0.0/33", "2001:db8::/129"}
	for _, in := range bad {
		if _, err := normalizeIPOrCIDR(in); err == nil && in != "" {
			t.Fatalf("expected error for %q", in)
		}
	}
}
