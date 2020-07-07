## Experiment Metadata

<table>
<tr>
<th> Name </th>
<th> Description </th>
<th> Documentation Link </th>
</tr>
<tr>
 <td> Pod Network Latency </td>
 <td> This experiment causes flaky access to application replica by injecting network delay using pumba. It injects latency on the specified container by starting a traffic control (tc) process with netem rules to add egress delays. It Can test the application's resilience to lossy/flaky network </td>
 <td>  <a href="https://docs.litmuschaos.io/docs/pod-network-latency/"> Here </a> </td>
 </tr>
 </table>

## Experiment CR for the pod-network-latency experiment

```
apiVersion: litmuschaos.io/v1alpha1
description:
  message: |
   Injects network latency on pods belonging to an app deployment
kind: ChaosExperiment
metadata:
  name: pod-network-latency
  version: 0.1.0
spec:
  definition:
    permissions:
      - apiGroups:
          - ""
          - "batch"
          - "litmuschaos.io"
        resources:
          - "jobs"
          - "pods"
          - "pods/log"
          - "events"
          - "chaosengines"
          - "chaosexperiments"
          - "chaosresults"
        verbs:
          - "create"
          - "delete"
          - "list"
          - "patch"
          - "update"
          - "get"
    image: "litmuschaos/go-runner:latest"
    args:
    - -c
    - ./experiments/pod-network-latency
    command:
    - /bin/bash
    env:
    - name: TOTAL_CHAOS_DURATION
      value: '60'
    - name: RAMP_TIME
      value: ''
    - name: TARGET_CONTAINER
      value: ''
    - name: NETWORK_INTERFACE
      value: 'eth0'
    - name: NETWORK_LATENCY
      value: '60000' # in ms
    - name: LIB
      value: 'pumba'      
    - name: LIB_IMAGE
      value: 'gaiaadm/pumba:0.6.5'
    labels:
      name: pod-network-latency

```