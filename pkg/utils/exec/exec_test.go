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
	tests := []struct {
		name      string
		pod       *apiv1.Pod
		container string
		wantErr   bool
	}{
		{
			name: "Pod not running",
			pod: &apiv1.Pod{
				Status: apiv1.PodStatus{Phase: apiv1.PodPending},
			},
			container: "container1",
			wantErr:   true,
		},
		{
			name: "Container not ready",
			pod: &apiv1.Pod{
				Status: apiv1.PodStatus{
					Phase: apiv1.PodRunning,
					ContainerStatuses: []apiv1.ContainerStatus{
						{Name: "container1", Ready: false},
					},
				},
			},
			container: "container1",
			wantErr:   true,
		},
		{
			name: "Healthy pod and container",
			pod: &apiv1.Pod{
				Status: apiv1.PodStatus{
					Phase: apiv1.PodRunning,
					ContainerStatuses: []apiv1.ContainerStatus{
						{Name: "container1", Ready: true},
					},
				},
			},
			container: "container1",
			wantErr:   false,
		},
		{
			name: "Container name not matching",
			pod: &apiv1.Pod{
				Status: apiv1.PodStatus{
					Phase: apiv1.PodRunning,
					ContainerStatuses: []apiv1.ContainerStatus{
						{Name: "other-container", Ready: true},
					},
				},
			},
			container: "container1",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPodStatus(tt.pod, tt.container)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkPodStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
