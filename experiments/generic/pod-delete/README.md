## Experiment CR for the pod-delete experiment

```
apiVersion: litmuschaos.io/v1alpha1
description:
  message: |
    Deletes a pod belonging to a deployment/statefulset/daemonset
kind: ChaosExperiment
metadata:
  name: pod-delete
  version: 0.1.0
spec:
  definition:
    permissions:
      - apiGroups:
          - ""
          - "apps"
          - "batch"
          - "litmuschaos.io"
        resources:
          - "deployments"
          - "jobs"
          - "pods"
          - "pods/log"
          - "events"
          - "configmaps"
          - "chaosengines"
          - "chaosexperiments"
          - "chaosresults"
        verbs:
          - "create"
          - "list"
          - "get"
          - "patch"
          - "update"
          - "delete"
      - apiGroups:
          - ""
        resources: 
          - "nodes"
        verbs:
          - "get"
          - "list"
    image: "litmuschaos/litmus-go:ci"
    args:
    - -c
    - ./experiments/pod-delete
    command:
    - /bin/bash
    env:
    - name: TOTAL_CHAOS_DURATION
      value: '30'
    - name: CHAOS_INTERVAL
      value: '10'
    - name: RAMP_TIME
      value: ''
    - name: LIB
      value: 'litmus'
    - name: FORCE
      value: ''
    - name: KILL_COUNT
      value: ''
    - name: LIB_IMAGE
      value: 'litmuschaos/pod-delete-helper:latest'
    labels:
      name: pod-delete

```