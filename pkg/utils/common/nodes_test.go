package common

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/litmuschaos/litmus-go/pkg/clients"
)

// newNodeListClients serves a fixed node list so GetNodeList runs against a real client-go
// clientset instead of a stub of its own selection logic.
func newNodeListClients(t *testing.T, nodeNames ...string) clients.ClientSets {
	t.Helper()
	list := apiv1.NodeList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "NodeList"}}
	for _, name := range nodeNames {
		list.Items = append(list.Items, apiv1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(&list); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	kubeClient, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return clients.ClientSets{KubeClient: kubeClient}
}

// TestGetNodeListAffectedPercentage checks that NODES_AFFECTED_PERC cannot select a node more than
// once. filterPodsByPercentage clamps the pod percentage to 100 before the rule of three;
// GetNodeList does not, so a percentage above 100 walks the node list in a circle and repeats it.
func TestGetNodeListAffectedPercentage(t *testing.T) {
	for _, tt := range []struct {
		name       string
		percentage int
		want       int
	}{
		{"half of three nodes", 50, 1},
		{"all three nodes", 100, 3},
		{"above one hundred cannot exceed the node count", 200, 3},
		{"far above one hundred cannot exceed the node count", 1000, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clientSets := newNodeListClients(t, "node-a", "node-b", "node-c")
			got, err := GetNodeList("", "", tt.percentage, clientSets)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want {
				t.Errorf("GetNodeList(perc=%d) returned %d nodes %v, want %d", tt.percentage, len(got), got, tt.want)
			}
			seen := map[string]bool{}
			for _, name := range got {
				if seen[name] {
					t.Errorf("GetNodeList(perc=%d) targeted %q more than once: %v", tt.percentage, name, got)
				}
				seen[name] = true
			}
		})
	}
}
