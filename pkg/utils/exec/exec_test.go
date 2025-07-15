// exec_test.go
package exec

import (
	"testing"

	apiv1 "k8s.io/api/core/v1"
)

func TestSetExecCommandAttributes(t *testing.T) {
	p := &PodDetails{}
	SetExecCommandAttributes(p, "test-pod", "test-container", "default")

	if p.PodName != "test-pod" || p.ContainerName != "test-container" || p.Namespace != "default" {
		t.Errorf("SetExecCommandAttributes failed to set values properly")
	}
}

func TestCheckPodStatus(t *testing.T) {
	// I am checking these three conditions - 
	
	// Pod not running
	pod1 := &apiv1.Pod{
		Status: apiv1.PodStatus{Phase: apiv1.PodPending},
	}
	err := checkPodStatus(pod1, "container1")
	if err == nil {
		t.Error("Expected error for non-running pod")
	}

	// Container not ready
	pod2 := &apiv1.Pod{
		Status: apiv1.PodStatus{
			Phase: apiv1.PodRunning,
			ContainerStatuses: []apiv1.ContainerStatus{
				{Name: "container1", Ready: false},
			},
		},
	}
	err = checkPodStatus(pod2, "container1")
	if err == nil {
		t.Error("Expected error for not-ready container")
	}

	// Healthy pod
	pod3 := &apiv1.Pod{
		Status: apiv1.PodStatus{
			Phase: apiv1.PodRunning,
			ContainerStatuses: []apiv1.ContainerStatus{
				{Name: "container1", Ready: true},
			},
		},
	}
	err = checkPodStatus(pod3, "container1")
	if err != nil {
		t.Errorf("Unexpected error for healthy pod: %v", err)
	}
}
