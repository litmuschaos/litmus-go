## Steps to Bootstrap a Chaos Experiment

The artefacts associated with a chaos-experiment are summarized below: 

- Submitted in the litmuschaos/litmus-go repository, under the experiments/*chaos-category* folder i.e, `experiments/generic`

  - Experiment business logic in golang. May involve creating new or reusing an existing chaoslib 
  - Experiment Kubernetes job (passes the experiment-specific chaos parameters, executes the business logic)

  Example: [pod delete experiment in litmus-go](/experiments/generic/pod-delete)

- Submitted in litmuschaos/chaos-charts repository, under the *chaos-category* folder i.e, `generic`

  - Experiment custom resource (CR) (holds experiment-specific chaos parameters & experiment entrypoint)
  - Experiment ChartServiceVersion (holds experiment metadata that will be rendered on [charthub](hub.litmuschaos.io))
  - Experiment RBAC (holds experiment-specific ServiceAccount, Role and RoleBinding)
  - Experiment Engine (holds experiment-specific chaosengine)

  Example: [pod delete experiment in chaos-charts](https://github.com/litmuschaos/chaos-charts/tree/master/charts/generic/pod-delete)

The *generate_experiment.go* script is a simple way to bootstrap your experiment, and helps create the aforementioned artefacts in the 
appropriate directory (i.e., as per the chaos-category) based on an attributes file provided as input by the chart-developer. The 
scaffolded files consist of placeholders which can then be filled as desired.  

### Pre-Requisites

- *go* should be is available.

### Steps to Generate Experiment Manifests

- Clone the litmus-go repository & navigate to the `contribute/developer-guide` folder

  ```
  $ git clone https://github.com/litmuschaos/litmus-go.git
  $ cd litmus/contribute/developer-guide
  ```

- Populate the `attributes.yaml` with details of the chaos experiment (or chart). Use the [attributes.yaml.sample](/contribute/developer-guide/attributes.yaml.sample) as reference. 

  As an example, let us consider an experiment to kill one of the replicas of a nginx deployment. The attributes.yaml can be constructed like this: 
  
  ```yaml
  $ cat attributes.yaml 
  
  ---
  name: "pod-delete"
version: "0.1.0"
category: "sample-category"
repository: "https://github.com/litmuschaos/litmus-go/tree/master/sample-category/pod-delete"
community: "https://kubernetes.slack.com/messages/CNXNB0ZTN"
description: "kills nginx pods in a random manner"
keywords:
  - "pods"
  - "kubernetes"
  - "sample-category"
  - "nginx"
scope: "Namespaced"
auxiliaryappcheck: false
permissions:
  - apigroups:
      - ""
      - "batch"
      - "litmuschaos.io"
    resources:
      - "jobs"
      - "pods"
      - "chaosengines"
      - "chaosexperiments"
      - "chaosresults"
    verbs:
      - "create"
      - "list"
      - "get"
      - "update"
      - "patch"
      - "delete"
maturity: "alpha"
maintainers:
  - name: "ksatchit"
    email: "ksatchit@mayadata.io"
provider:
  name: "Mayadata"
minkubernetesversion: "1.12.0"
references:
  - name: Documentation
    url: "https://docs.litmuschaos.io/docs/getstarted/"

  ```

- Run the following command to generate the necessary artefacts for submitting the `sample-category` chaos chart with 
  `pod-delete` experiment.

  ```
  $ go run generate_experiment.go -attributes=attributes.yaml -generateType=experiment
  ```

  **Note**: In the `-generateType` attribute, select the appropriate type of manifests to be generated, where, 
  - `chart`: Just the chaos-chart metadata, i.e., chartserviceversion yaml 
  - `experiment`: Chaos experiment artefacts belonging to a an existing OR new chart. 

  View the generated files in `/experiments/chaos-category` folder.

  ```
  $ cd /experiments

  $ ls -ltr

  total 8
  drwxr-xr-x 3 shubham shubham 4096 May 15 12:02 generic/
  drwxr-xr-x 3 shubham shubham 4096 May 15 13:26 sample-category/


  $ ls -ltr sample-category/

  total 12
  -rw-r--r-- 1 shubham shubham   41 May 15 13:26 sample-category.package.yaml
  -rw-r--r-- 1 shubham shubham  734 May 15 13:26 sample-category.chartserviceversion.yaml
  drwxr-xr-x 2 shubham shubham 4096 May 15 13:26 pod-delete/

  $ ls -ltr sample-category/pod-delete

  total 28
  -rw-r--r-- 1 shubham shubham  791 May 15 13:26 rbac.yaml
  -rw-r--r-- 1 shubham shubham  734 May 15 13:26 pod-delete.chartserviceversion.yaml
  -rw-r--r-- 1 shubham shubham  792 May 15 13:26 experiment.yaml
  -rw-r--r-- 1 shubham shubham 1777 May 15 13:26 pod-delete-k8s-job.yml
  -rw-r--r-- 1 shubham shubham 4533 May 15 13:26 pod-delete.go
  -rw-r--r-- 1 shubham shubham  813 May 15 13:26 engine.yaml
  
  ```
 
- Proceed with construction of business logic inside the `pod-delete.go` file, by making
  the appropriate modifications listed below to achieve the desired effect: 

  - variables 
  - entry & exit criteria checks for the experiment 
  - helper utils in either [pkg](/pkg/) or new [base chaos libraries](/chaoslib) 

- Update the `experiment.yaml` with the right chaos params in the `spec.definition.env` with their
  default values

- Update the `chaoslib/litmus/pod-delete/pod-delete.go` chaoslib to achieve the desired effect or reuse the existing chaoslib.

- Create an experiment README explaining, briefly, the *what*, *why* & *how* of the experiment to aid users of this experiment. 

### Steps to Test Experiment 

The experiment created using the above steps, can be tested in the following manner: 

- Run the `pod-delete-k8s-job.yml` with the desired values in the ENV and appropriate `chaosServiceAccount` 
  using a custom dev image instead of `litmuschaos/litmus-go` (say, ksatchit/litmus-go) that packages the 
  business logic.

- (Optional) Once the experiment has been validated using the above step, it can also be tested against the standard chaos 
  workflow using the `experiment.yaml`. This involves: 

  - Launching the Chaos-Operator
  - Creating the ChaosExperiment CR on the cluster (use the same custom dev image used in the above step) 
  - Creating the ChaosEngine to execute the above ChaosExperiment
  - Verifying the experiment status via ChaosResult 

  Refer [litmus docs](https://docs.litmuschaos.io/docs/getstarted/) for more details on this procedure.

### Steps to Include the Chaos Charts/Experiments into the ChartHub

- Send a PR to the [litmus-go](https://github.com/litmuschaos/litmus-go) repo with the modified experiment files
- Send a PR to the [chaos-charts](https://github.com/litmuschaos/chaos-charts) repo with the modified experiment CR, 
  experiment chartserviceversion, chaos chart (category-level) chartserviceversion & package (if applicable) YAMLs
