## Steps to Bootstrap a Chaos Experiment

The artifacts associated with a chaos-experiment are summarized below: 

- Submitted in the litmuschaos/litmus-go repository, under the experiments/*chaos-category*/*experiment-name* folder 

  - Experiment business logic in golang. May involve creating new or reusing an existing chaoslib 
  - Experiment test deployment manifest that is used for verification purposes
  - Experiment RBAC (holds experiment-specific ServiceAccount, Role and RoleBinding)

  Example: [pod delete experiment in litmus-go](/experiments/generic/pod-delete)

- Submitted in litmuschaos/chaos-charts repository, under the *chaos-category* folder

  - Experiment custom resource (CR) (holds experiment-specific chaos parameters & experiment entrypoint)
  - Experiment ChartServiceVersion (holds experiment metadata that will be rendered on [charthub](https://hub.litmuschaos.io/))
  - Experiment RBAC (holds experiment-specific ServiceAccount, Role and RoleBinding)
  - Experiment Engine (holds experiment-specific chaosengine)

  Example: [pod delete experiment in chaos-charts](https://github.com/litmuschaos/chaos-charts/tree/master/charts/generic/pod-delete)

The *generate_experiment.go* script is a simple way to bootstrap your experiment, and helps create the aforementioned artifacts in the 
appropriate directory (i.e., as per the chaos-category) based on an attributes file provided as input by the chart-developer. The 
scaffolded files consist of placeholders which can then be filled as desired.  

### Pre-Requisites

- *go* should be is available & the GOPATH env is configured appropriately

### Steps to Generate Experiment Manifests

- Clone the litmus-go repository & navigate to the `contribute/developer-guide` folder

  ```
  $ git clone https://github.com/litmuschaos/litmus-go.git
  $ cd litmus-go/contribute/developer-guide
  ```

- Populate the `attributes.yaml` with details of the chaos experiment (or chart). Use the [attributes.yaml.sample](/contribute/developer-guide/attributes.yaml.sample) as reference. 

  As an example, let us consider an experiment to kill one of the replicas of a nginx deployment. The attributes.yaml can be constructed like this: 
  
  ```yaml
  $ cat attributes.yaml 
  
  ---
  name: "sample-exec-chaos"
  version: "0.1.0"
  category: "sample-category"
  repository: "https://github.com/litmuschaos/litmus-go/tree/master/sample-category/sample-exec-chaos"
  community: "https://kubernetes.slack.com/messages/CNXNB0ZTN"
  description: "it execs inside target pods to run the chaos inject commands, waits for the chaos duration and reverts the chaos"
  keywords:
    - "pods"
    - "kubernetes"
    - "sample-category"
    - "exec"
  platforms:
    - Minikube
  scope: "Namespaced"
  auxiliaryappcheck: false
  permissions:
    - apigroups:
        - ""
        - "batch"
        - "apps"
        - "litmuschaos.io"
      resources:
        - "jobs"
        - "pods"
        - "pods/log"
        - "events"
        - "deployments"
        - "replicasets"
        - "pods/exec"
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
        - "deletecollection"
  maturity: "alpha"
  maintainers:
    - name: "ispeakc0de"
      email: "shubham@chaosnative.com" 
  provider:
    name: "ChaosNative"
  minkubernetesversion: "1.12.0"
  references:
    - name: Documentation
      url: "https://docs.litmuschaos.io/docs/getstarted/"

  ```

- Run the following command to generate the necessary artifacts for submitting the `sample-category` chaos chart with 
  `sample-exec-chaos` experiment.

  ```
  $ ./litmus-sdk generate <generate-type> -f=attributes.yaml
  ```

  **Note**: Replace the `<generate-type>` placeholder with the appropriate value based on the usecase: 
  - `experiment`: Chaos experiment artifacts belonging to an existing OR new experiment.
  - `chart`: Just the chaos-chart metadata, i.e., chartserviceversion.yaml
      - Provide the type of chart in the `-t` flag. It supports the following values:
           - `category`: It creates the chart metadata for the category i.e chartserviceversion, package manifests
           - `experiment`: It creates the chart for the experiment i.e chartserviceversion, engine, rbac, experiment manifests
           - `all`: it creates both category and experiment charts (default type)

  - Provide the path of the attribute.yaml manifest in the `-f` flag.

  View the generated files in `/experiments/<chaos-category>` folder.

  ```
  $ cd /experiments

  $ ls -ltr

  total 8
  drwxr-xr-x 3 shubham shubham 4096 June 10 12:02 generic/
  drwxr-xr-x 3 shubham shubham 4096 June 10 13:26 sample-category/


  $ ls -ltr sample-category/

  total 4
  drwxr-xr-x 5 shubham shubham 4096 June 10 13:26 sample-exec-chaos/

  $ ls -ltr sample-category/sample-exec-chaos/

  total 12
  drwxr-xr-x 3 shubham shubham 4096 Jun 10 22:41 charts/
  drwxr-xr-x 2 shubham shubham 4096 Jun 10 22:41 test/
  drwxr-xr-x 2 shubham shubham 4096 Jun 10 22:41 experiment/

  $ ls -ltr sample-category/sample-exec-chaos/test

  total 4
  -rw-r--r-- 1 shubham shubham  1039 June 10 13:26 test.yaml

  $ ls -ltr sample-category/sample-exec-chaos/experiment

  total 12 
  -rw-r--r-- 1 shubham shubham 8893 Jun 10 22:41 sample-exec-chaos.go

  $ ls -ltr sample-category/sample-exec-chaos/charts

  total 28
  -rw-r--r-- 1 shubham shubham  152 Jun 10 22:41 sample-category.package.yaml
  -rw-r--r-- 1 shubham shubham  869 Jun 10 22:41 sample-category.chartserviceversion.yaml
  -rw-r--r-- 1 shubham shubham  999 Jun 10 22:41 sample-exec-chaos.chartserviceversion.yaml
  -rw-r--r-- 1 shubham shubham 1534 Jun 10 22:41 experiment.yaml
  -rw-r--r-- 1 shubham shubham 1209 Jun 10 22:41 rbac.yaml
  -rw-r--r-- 1 shubham shubham  735 Jun 10 22:41 engine.yaml
  drwxr-xr-x 2 shubham shubham 4096 Jun 10 22:41 icons/
  ```
 
- Proceed with construction of business logic inside the `sample-exec-chaos.go` file, by making
  the appropriate modifications listed below to achieve the desired effect: 

  - variables 
  - entry & exit criteria checks for the experiment 
  - helper utils in either [pkg](/pkg/) or new [base chaos libraries](/chaoslib) 


- The chaoslib is created at `chaoslib/litmus/sample-exec-chaos/lib/sample-exec-chaos.go` path. It contains some pre-defined steps which runs the `ChaosInject` command (explicitly provided as an ENV var in the experiment CR). Which will induce chaos in the target application. It will wait for the given chaos duration and finally runs the `ChaosKill` command (also provided as an ENV var) for cleanup purposes. Update this chaoslib to achieve the desired effect based on the use-case or reuse the other existing chaoslib.

- Create an experiment README explaining, briefly, the *what*, *why* & *how* of the experiment to aid users of this experiment. 

### Steps to Test Experiment 

We can use [Okteto](https://github.com/okteto/okteto) to help us in performing the dev-tests for experiment created. 
Follow the steps provided below to setup okteto & test the experiment execution. 

- Install the Okteto CLI 

  ```
  curl https://get.okteto.com -sSfL | sh
  ```

- (Optional) Create a sample nginx deployment that can be used as the application under test (AUT).

  ```
  kubectl create deployment nginx --image=nginx
  ```

- Setup the RBAC necessary for execution of this experiment by applying the generated `rbac.yaml`

  ```
  kubectl apply -f rbac.yaml
  ```

- Modify the `test/test.yaml` with the desired values (app & chaos info) in the ENV and appropriate `chaosServiceAccount` along 
  with any other dependencies, if applicable (configmaps, volumes etc.,) & create this deployment  

  ```
  kubectl apply -f test/test.yml
  ```

- Go to the root of this repository (litmuschaos/litmus-go) & launch the Okteto development environment in your workspace.
  This should take you to the bash prompt on the dev container into which the content of the litmus-go repo is loaded. 

  ```
  root@test:~/okteto/litmus-go# okteto up 

  Deployment litmus-go doesn't exist in namespace litmus. Do you want to create a new one? [y/n]: y
  ✓  Development container activated
  ✓  Files synchronized

  The value of /proc/sys/fs/inotify/max_user_watches in your cluster nodes is too low.
  This can affect file synchronization performance.
  Visit https://okteto.com/docs/reference/known-issues/index.html for more information.
      Namespace: default
      Name:      litmus-experiment
      Forward:   2345 -> 2345
                 8080 -> 8080

  Welcome to your development container. Happy coding!
  ```

  This dev container inherits the env, serviceaccount & other properties specified on the test deployment & is now suitable for 
  running the experiment.

- Execute the experiment against the sample app chosen & verify the steps via logs printed on the console.

  ```
  go run bin/go-runner.go -name <experiment-name>
  ``` 

- In parallel, observe the experiment execution via the changes to the pod/node state

  ```
  watch -n 1 kubectl get pods,nodes
  ```

- If there are necessary changes to the code based on the run, make them via your favourite IDE. 
  These changes are automatically reflected on the dev container. Re-run the experiment to confirm changes. 

- Once the experiment code is validated, stop/remove the development environment 


  ```
  root@test:~/okteto/litmus-go# okteto down
  ✓  Development container deactivated
  i  Run 'okteto push' to deploy your code changes to the cluster
  ```

- (Optional) Once the experiment has been validated using the above step, it can also be tested against the standard Litmus chaos 
  flow. This involves: 

  - Creating a custom image built with the code validated by the previous steps
  - Launching the Chaos-Operator
  - Modifying the ChaosExperiment manifest (experiment.yaml) with right defaults (env & other attributes, as applicable) & creating 
    this CR on the cluster (pointing the `.spec.definition.image` to the custom one just built)
  - Modifying the ChaosEngine manifest (engine.yaml) with right app details, run properties & creating this CR to launch the chaos pods
  - Verifying the experiment status via ChaosResult 

  Refer [litmus docs](https://docs.litmuschaos.io/docs/getstarted/) for more details on performing each step in this procedure.

### Steps to Include the Chaos Charts/Experiments into the ChartHub

- Send a PR to the [litmus-go](https://github.com/litmuschaos/litmus-go) repo with the modified experiment files, rbac, 
  test deployment & README.
- Send a PR to the [chaos-charts](https://github.com/litmuschaos/chaos-charts) repo with the modified experiment CR, 
  experiment chartserviceversion, rbac, (category-level) chaos chart chartserviceversion & package.yaml (if applicable). 
