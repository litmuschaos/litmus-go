# Run E2E tests using GitHub Chaos Actions 

- When you commit code to your repository, you can continuously build and test the code to make sure that the commit doesn't introduce errors. The error could be in the form of some security issue, functional issue, or performance issue which can be tested using different custom tests, linters, or by pulling actions. This brings the need of having *Chaos Actions* which will perform a chaos test on the application over a particular commit which in-turn helps to track the performance of the application on a commit level. This can be done by commenting on the Pull Request. 

## Through comments on PR

- We can run tests for any desired experiment or set of experiments by just commenting on the Pull Request. The format of comment will be:

```bash

/run-e2e-<test-name/test-group>

```

_Experiments Available for custom bot:_

<table style="width:100%">
  <tr>
    <th>Resource chaos</th>
    <th>Network Chaos</th>
    <th>IO Chaos</th>
    <th>Others</th>    
  </tr>
  <tr>
    <td>pod-cpu-hog</td>
    <td>pod-network-latency</td>
    <td>node-io-stress</td>
    <td>pod-delete</td>    
  </tr>
  <tr>
    <td>pod-memory-hog</td>
    <td>pod-network-loss</td>
    <td></td>
    <td>container-kill</td>    
  </tr>
  <tr>
    <td>node-cpu-hog</td>
    <td>pod-network-corruption</td>
    <td></td>
    <td>pod-autoscaler</td>    
  </tr>
  <tr>
    <td>node-memory-hog</td>
    <td>pod-network-duplication</td>
    <td></td>
    <td></td>    
  </tr>  
</table>

### Group Tests

<table style="width:100%">
  <tr>
    <th>Command</th>
    <th>Description</th>
  </tr>
  <tr>
    <td><code>/run-e2e-all</code></td>
    <td>Runs all available tests. This includes all resource chaos test, network chaos test, IO test and other tests. It will update the comment if it gets passed.</td>
  </tr>
  <tr>
    <td><code>/run-e2e-network-chaos</code></td>
    <td>Runs all network chaos tests. This includes pod network corruption, pod network duplication, pod network loss, pod network latency.</td>
  </tr> 
  <tr>
    <td><code>/run-e2e-resource-chaos</code></td>
    <td>Runs all resource chaos tests. This includes pod level cpu and memory chaos test and node level cpu and memory chaos test.</td>
  </tr>   
  <tr>
    <td><code>/run-e2e-io-chaos</code></td>
    <td>Runs all io chaos tests. Currently it only includes node io stress</td>
  </tr>   
</table>

### Individual Tests

<table style="width:100%">
  <tr>
    <th>Command</th>
    <th>Description</th>
  </tr>
   <tr>
    <td><code>/run-e2e-pod-delete</code></td>
    <td>Runs pod delete chaos test using GitHub chaos action which fail the application pod</td>
  </tr>
   <tr>
    <td><code>/run-e2e-container-kill</code></td>
    <td>Runs container kill experiment using GitHub chaos action which kill containers on the application pod</td>
  </tr> 
   <tr>
    <td><code>/run-e2e-pod-cpu-hog</code></td>
    <td>Runs pod level CPU chaos experiment using GitHub chaos action which consume CPU resources on the application container</td>
  </tr>   
  <tr>
    <td><code>/run-e2e-pod-memory-hog</code></td>
    <td>Runs pod level memory chaos test which consume memory resources on the application container</td>
  </tr>   
  <tr>
    <td><code>/run-e2e-node-cpu-hog</code></td>
    <td>Runs node level cpu chaos test which exhaust CPU resources on the Kubernetes Node </td>
  </tr>   
  <tr>
    <td><code>/run-e2e-node-memory-hog</code></td>
    <td>Runs node level memory chaos test which exhaust CPU resources on the Kubernetes Node</td>
  </tr>  
  <tr>
    <td><code>/run-e2e-node-io-stress</code></td>
    <td>Runs pod level memory chaos test which gives IO stress on the Kubernetes Node </td>
  </tr>

  <tr>
    <td><code>/run-e2e-pod-network-corruption<code></td>
    <td>Run pod-network-corruption test which inject network packet corruption into application pod</td>
  </tr>
  <tr>
    <td><code>/run-e2e-pod-network-latency</code></td>
    <td>Run pod-network-latency test which inject network packet latency into application pod </td>
  </tr>
   <tr>
    <td><code>/run-e2e-pod-network-loss</code></td>
    <td>Run pod-network-loss test which inject network packet loss into application pod </td>
  </tr>
  <tr>
    <td><code>/run-e2e-pod-network-duplication</code></td>
    <td>Run pod-network-duplication test which inject network packet duplication into application pod </td>
  </tr>

</table>


***Note:*** *All the tests are performed on a KinD cluster with containerd runtime.*
