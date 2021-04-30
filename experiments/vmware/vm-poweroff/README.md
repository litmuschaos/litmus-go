## Experiment Metadata

<table>
  <tr>
    <th> Type </th>
    <th> Description  </th>
    <th> Tested K8s Platform </th>
  </tr>
  <tr>
    <td> VMWare </td>
    <td> Stopping a VM for a certain chaos duration</td>
    <td> EKS </td>
  </tr>
</table>


## Prerequisites

- Ensure that Kubernetes Version > 1.13
- Ensure that Vcenter Version is 6.X (Support for 7.X will be added soon)
- Ensure that the Litmus Chaos Operator is running by executing `kubectl get pods` in operator namespace (typically, `litmus`). If not, install from [here](https://docs.litmuschaos.io/docs/getstarted/#install-litmus)
- Ensure that the `vm-poweroff` experiment resource is available in the cluster by executing `kubectl get chaosexperiments` in the desired namespace If not, install from [here](https://hub.litmuschaos.io/api/chaos/master?file=charts/vmware/vm-poweroff/experiment.yaml)
- Ensure that you have sufficient Vcenter access to stop and start the vm. 
- Ensure to create a Kubernetes secret having the Vcenter credentials in the `CHAOS_NAMESPACE`. A sample secret file looks like:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vcenter-secret
  namespace: litmus
type: Opaque
stringData:
    VCENTERSERVER: XXXXXXXXXXX
    VCENTERUSER: XXXXXXXXXXXXX
    VCENTERPASS: XXXXXXXXXXXXX
```


## Entry-Criteria

-   vm is healthy before chaos injection.

## Exit-Criteria

-   vm is healthy post chaos injection.

## Details

-   Experiment uses vmware api's to start/stop the vm. 
-   Stops a VM before bringing it back to running state after the specified chaos duration. 
-   It helps to check the performance of the application/process running on the vmware server.


## Integrations

-   vm-poweroff can be effected using the chaos library: `litmus`, which makes use of vmware api's to start/stop a vm in vmware environment. 
-   The desired chaoslib can be selected by setting the above options as value for the env variable `LIB`

## Steps to Execute the Chaos Experiment

- This Chaos Experiment can be triggered by creating a ChaosEngine resource on the cluster. To understand the values to provide in a ChaosEngine specification, refer [Getting Started](getstarted.md/#prepare-chaosengine)

- Follow the steps in the sections below to create the chaosServiceAccount, prepare the ChaosEngine & execute the experiment.

### Prepare chaosServiceAccount

- Use this sample RBAC manifest to create a chaosServiceAccount in the desired (app) namespace. This example consists of the minimum necessary role permissions to execute the experiment.

#### Sample Rbac Manifest

[embedmd]:# (https://raw.githubusercontent.com/litmuschaos/chaos-charts/master/charts/vmware/vm-poweroff/rbac.yaml yaml)
```yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: vm-poweroff-sa
  namespace: default
  labels:
    name: vm-poweroff-sa
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: Role
metadata:
  name: vm-poweroff-sa
  namespace: default
  labels:
    name: vm-poweroff-sa
rules: [{'apiGroups': ['', 'batch', 'litmuschaos.io'], 'resources': ['jobs', 'pods', 'deployments','pods/log', 'events', 'chaosengines', 'chaosexperiments', 'chaosresults', 'secrets'], 'verbs': ['create', 'list', 'get', 'update', 'patch', 'delete','deletecollection']}]
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: RoleBinding
metadata:
  name: vm-poweroff-sa
  namespace: default
  labels:
    name: vm-poweroff-sa
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: vm-poweroff-sa
subjects:
- kind: ServiceAccount
  name: vm-poweroff-sa
  namespace: default
```

### Prepare ChaosEngine

- Provide the application info in `spec.appinfo`
- Provide the auxiliary applications info (ns & labels) in `spec.auxiliaryAppInfo`
- Override the experiment tunables if desired in `experiments.spec.components.env`
- To understand the values to provided in a ChaosEngine specification, refer [ChaosEngine Concepts](chaosengine-concepts.md)

#### Supported Experiment Tunables

<table>
  <tr>
    <th> Variables </th>
    <th> Description </th>
    <th> Specify In ChaosEngine </th>
    <th> Notes </th>
	<th> How to get</th>
  </tr>
  <tr>
    <td> APP_VM_MOID </td>
    <td> Moid of the vmware instance</td>
    <td> Required </td>
    <td> </td>
	<td> Once you open VM in vCenter WebClient, you can find MOID in address field (VirtualMachine:vm-5365). Eg: vm-5365 </td>
  </tr>
  <tr>
    <td> TOTAL_CHAOS_DURATION </td>
    <td> The time duration for chaos insertion (sec) </td>
    <td> Optional </td>
    <td> Defaults to 30s </td>
	<td> </td>
  </tr>
  <tr>
    <td> VCENTERSERVER </td>
    <td> IP Address of the vcenter </td>
    <td> Required </td>
    <td> Should be specified in secret created </td>
	<td> IP Address/DNS of Vcenter server</td>
  </tr>
  <tr>
    <td> VCENTERUSER </td>
    <td> Username of Vcenter </td>
    <td> Required </td>
    <td> Should be specified in secret created having access to start/stop the vm </td>
	<td> </td>
  </tr>
  <tr>
    <td> VCENTERPASS </td>
    <td> Password of Vcenter </td>
    <td> Required </td>
    <td> Should be specified in secret created </td>
	<td> </td>
  </tr>
</table>

#### Sample ChaosEngine Manifest

[embedmd]:# (https://raw.githubusercontent.com/litmuschaos/chaos-charts/master/charts/vmware/vm-poweroff/engine.yaml yaml)
```yaml
apiVersion: litmuschaos.io/v1alpha1
kind: ChaosEngine
metadata:
  name: nginx-chaos
  namespace: default
spec:
  # It can be true/false
  annotationCheck: 'false'
  # It can be active/stop
  engineState: 'active'
  #ex. values: ns1:name=percona,ns2:run=nginx
  auxiliaryAppInfo: ''
  chaosServiceAccount: vm-poweroff-sa
  monitoring: false
  # It can be delete/retain
  jobCleanUpPolicy: 'retain'
  experiments:
    - name: vm-poweroff
      spec:
        components:
          env:
            # set chaos duration (in sec) as desired
            - name: TOTAL_CHAOS_DURATION
              value: '60'

            # provide the vm moid
            - name: APP_VM_MOID
              value: ''              

            - name: VCENTERSERVER
              valueFrom:
                secretKeyRef:
                  name: vcenter-secret
                  key: VCENTERSERVER

            - name: VCENTERUSER
              valueFrom:
                secretKeyRef:
                  name: vcenter-secret
                  key: VCENTERUSER

            - name: VCENTERPASS
              valueFrom:
                secretKeyRef:
                  name: vcenter-secret
                  key: VCENTERPASS        
```

### Create the ChaosEngine Resource

- Create the ChaosEngine manifest prepared in the previous step to trigger the Chaos.

  `kubectl apply -f chaosengine.yml`

- If the chaos experiment is not executed, refer to the [troubleshooting](https://docs.litmuschaos.io/docs/faq-troubleshooting/) 
  section to identify the root cause and fix the issues.

### Watch Chaos progress
  
-  You can monitor vcenter console to keep a watch over the vm state.   

### Check Chaos Experiment Result

- Check whether the application is resilient to the vm-poweroff, once the experiment (job) is completed. The ChaosResult resource name is derived like this: `<ChaosEngine-Name>-<ChaosExperiment-Name>`.

  `kubectl describe chaosresult nginx-chaos-vm-poweroff -n <application-namespace>`

### vm-poweroff Experiment Demo

- A sample recording of this experiment execution will be added soon.
