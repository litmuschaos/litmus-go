## Experiment Metadata

<table>
<tr>
<th> Name </th>
<th> Description </th>
<th> Documentation Link </th>
</tr>
<tr>
 <td> Pod Network Duplication </td>
 <td> This experiment causes network duplication using pumba. It injects network duplication on the specified container by starting a traffic control (tc) process with netem rules. It Can test the application's resilience to duplicate network </td>
 <td>  <a href="https://docs.litmuschaos.io/docs/pod-network-duplication/"> Here </a> </td>
 </tr>
 </table>

## Experiment CR for the pod-network-duplication experiment

```
apiVersion: litmuschaos.io/v1alpha1
description:
  message: |
    Deletes a pod belonging to a deployment/statefulset/daemonset
kind: ChaosExperiment
metadata:
  name: pod-network-duplication
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
    - ./experiments/pod-network-duplication
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
    - name: NETWORK_PACKET_DUPLICATION_PERCENTAGE
      value: '100' # in percentage
    - name: LIB
      value: 'pumba'      
    - name: LIB_IMAGE
      value: 'gaiaadm/pumba:0.6.5'
    labels:
      name: pod-network-duplication

```