## Experiment CR for the pod-autoscaler experiment

```
apiVersion: litmuschaos.io/v1alpha1
description:
  message: |
    Scale the deployment replicas to check the autoscaling capability
kind: ChaosExperiment
metadata:
  name: pod-autoscaler
  version: 0.1.0
spec:
  definition:
    scope: Namespaced
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
    imagePullPolicy: Always
    args:
    - -c
    - ./experiments/pod-autoscaler
    command:
    - /bin/bash
    env:

    - name: TOTAL_CHAOS_DURATION
      value: '60'

    - name: REPLICA_COUNT
      value: '5'

    # Period to wait before and after injection of chaos in sec
    - name: RAMP_TIME
      value: ''

    - name: LIB
      value: 'litmus'    
    labels:
      name: pod-autoscaler

```