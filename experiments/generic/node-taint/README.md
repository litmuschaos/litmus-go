## Experiment CR for the node-taint experiment

```
apiVersion: litmuschaos.io/v1alpha1
description:
  message: |
    Taint the node where application pod is scheduled
kind: ChaosExperiment
metadata:
  name: node-taint
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
    - ./experiments/node-taint
    command:
    - /bin/bash
    env:
     # provide taint label & effect
     # sample input is - key=value:effect
    - name: TAINTS
      value: ''
    - name: RAMP_TIME
      value: ''
    - name: LIB
      value: 'litmus'
    - name: APP_NODE
      value: ''
    labels:
      name: node-taint
```