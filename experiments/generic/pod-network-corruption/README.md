## Experiment Metadata

<table>
<tr>
<th> Name </th>
<th> Description </th>
<th> Documentation Link </th>
</tr>
<tr>
 <td> Pod Network Corruption </td>
 <td> This chaos action Injects packet corruption on the specified container by starting a traffic control (tc) process with netem rules to add egress packet corruption. Corruption is injected via pumba library with command Pumba netem corruption by passing the relevant network interface, packet-corruption-percentage, chaos duration, and regex filter for the container name. </td>
 <td>  <a href="https://docs.litmuschaos.io/docs/pod-network-corruption/"> Here </a> </td>
 </tr>
 </table>

## Experiment CR for the pod-network-corruption experiment

```
apiVersion: litmuschaos.io/v1alpha1
description:
  message: |
   Injects network corruption on pods belonging to an app deployment
kind: ChaosExperiment
metadata:
  name: pod-network-corruption
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
    - ./experiments/pod-network-corruption
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
    - name: NETWORK_PACKET_CORRUPTION_PERCENTAGE
      value: '100' # in percentage
    - name: LIB
      value: 'pumba'      
    - name: LIB_IMAGE
      value: 'gaiaadm/pumba:0.6.5'
    labels:
      name: pod-network-corruption

```