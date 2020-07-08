## Experiment CR for the node-drain experiment

```
apiVersion: litmuschaos.io/v1alpha1
description:
  message: |
    Drain the node where application pod is scheduled
kind: ChaosExperiment
metadata:
  name: node-drain
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
    image: "litmuschaos/go-runner:ci"
    args:
    - -c
    - ./experiments/node-drain
    command:
    - /bin/bash
    env:
    - name: TOTAL_CHAOS_DURATION
      value: '60'
    - name: RAMP_TIME
      value: ''
    - name: LIB
      value: 'litmus'
    - name: APP_NODE
      value: ''
    labels:
      name: node-drain

```