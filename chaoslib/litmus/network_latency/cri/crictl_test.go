package cri

import "testing"

func TestCrictl(t *testing.T) {

	// crictl inspect output
	json := `
{
  "status": {
    "id": "c739a31ab698e6e1c679442a538d16cc7199703c80f030e159b5de6b46e60518",
    "metadata": {
      "attempt": 0,
      "name": "nginx-unprivileged"
    },
    "state": "CONTAINER_RUNNING",
    "createdAt": "2020-07-28T16:50:35.84027013Z",
    "startedAt": "2020-07-28T16:50:35.996159402Z",
    "finishedAt": "1970-01-01T00:00:00Z",
    "exitCode": 0,
    "image": {
      "image": "docker.io/nginxinc/nginx-unprivileged:latest"
    },
    "imageRef": "docker.io/nginxinc/nginx-unprivileged@sha256:0fd19475c17fff38191ef0dd3d1b949a25fd637cd64756146cc99363e580cf3a",
    "reason": "",
    "message": "",
    "labels": {
      "io.kubernetes.container.name": "nginx-unprivileged",
      "io.kubernetes.pod.name": "app-7f99cf5459-gdqw7",
      "io.kubernetes.pod.namespace": "myteam",
      "io.kubernetes.pod.uid": "d2368c41-679f-40a8-aa5d-6a763876ef06"
    },
    "annotations": {
      "io.kubernetes.container.hash": "ddf9b623",
      "io.kubernetes.container.restartCount": "0",
      "io.kubernetes.container.terminationMessagePath": "/dev/termination-log",
      "io.kubernetes.container.terminationMessagePolicy": "File",
      "io.kubernetes.pod.terminationGracePeriod": "30"
    },
    "mounts": [
      {
        "containerPath": "/etc/hosts",
        "hostPath": "/var/lib/kubelet/pods/d2368c41-679f-40a8-aa5d-6a763876ef06/etc-hosts",
        "propagation": "PROPAGATION_PRIVATE",
        "readonly": false,
        "selinuxRelabel": false
      },
      {
        "containerPath": "/dev/termination-log",
        "hostPath": "/var/lib/kubelet/pods/d2368c41-679f-40a8-aa5d-6a763876ef06/containers/nginx-unprivileged/00467287",
        "propagation": "PROPAGATION_PRIVATE",
        "readonly": false,
        "selinuxRelabel": false
      },
      {
        "containerPath": "/var/run/secrets/kubernetes.io/serviceaccount",
        "hostPath": "/var/lib/kubelet/pods/d2368c41-679f-40a8-aa5d-6a763876ef06/volumes/kubernetes.io~secret/default-token-8lf4k",
        "propagation": "PROPAGATION_PRIVATE",
        "readonly": true,
        "selinuxRelabel": false
      }
    ],
    "logPath": "/var/log/pods/myteam_app-7f99cf5459-gdqw7_d2368c41-679f-40a8-aa5d-6a763876ef06/nginx-unprivileged/0.log"
  },
  "pid": 72496,
  "sandboxId": "e978d37294a29c4a7f3f668f44f33431d4b9b892e415fcddfcdf71a8d047a2f7"
}`

	expectedPID := 72496
	PID, err := parsePIDFromJson([]byte(json))
	if err != nil {
		t.Fatalf("Fail to parse json: %s", err)
	}
	if PID != expectedPID {
		t.Errorf("Fail to parse PID from json. Expected %d, got %d", expectedPID, PID)
	}

}
