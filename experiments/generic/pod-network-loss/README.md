## Experiment Metadata

<table>
<tr>
<th> Name </th>
<th> Description </th>
<th> Documentation Link </th>
</tr>
<tr>
 <td> Pod Network Loss </td>
 <td> This experiment injects chaos to disrupt network connectivity to kubernetes pods.The application pod should be healthy once chaos is stopped. It causes loss of access to application replica by injecting packet loss using pumba </td>
 <td>  <a href="https://docs.litmuschaos.io/docs/pod-network-loss/"> Here </a> </td>
 </tr>
 </table>

## Experiment CR for the pod-network-loss experiment

```
apiVersion: litmuschaos.io/v1alpha1
description:
  message: |
   Injects network loss on pods belonging to an app deployment
kind: ChaosExperiment
metadata:
  name: pod-network-loss
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
    - ./experiments/pod-network-loss
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
    - name: NETWORK_PACKET_LOSS_PERCENTAGE
      value: '100' # in percentage
    - name: LIB
      value: 'pumba'      
    - name: LIB_IMAGE
      value: 'gaiaadm/pumba:0.6.5'
    labels:
      name: pod-network-loss

```