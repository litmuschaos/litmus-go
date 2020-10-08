package cmd

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"github.com/litmuschaos/litmus-go/contribute/developer-guide/types"
	"gopkg.in/yaml.v2"
)

// GenerateExperiment ...
func GenerateExperiment(attributeFile, generationType *string) {

	// Fetch all the required attributes from the given file
	// Experiment contains all the required attributes
	var experimentDetails types.Experiment
	GetConfig(&experimentDetails, *attributeFile)

	// getting the current directory name
	currDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	// generating the parent directory name
	// so that it can be utilise to get the relative path of all files from there
	currDir = path.Join(currDir, "..")
	litmusRootDir := filepath.Dir(currDir)

	// creating the directory of experiment category, if not present
	chartDIR := litmusRootDir + "/experiments/" + experimentDetails.Category
	CreateDirectoryIfNotPresent(chartDIR)

	if *generationType == "chart" {
		csvFilePath := chartDIR + "/" + experimentDetails.Category + ".chartserviceversion.yaml"
		GenerateFile(experimentDetails, csvFilePath, "./templates/chartserviceversion.tmpl")
		packageFilePath := chartDIR + "/" + experimentDetails.Category + ".package.yaml"
		GenerateFile(experimentDetails, packageFilePath, "./templates/package.tmpl")
	} else if *generationType == "experiment" {

		// creating the directory for the experiment, if not present
		experimentDIR := chartDIR + "/" + experimentDetails.Name
		CreateDirectoryIfNotPresent(experimentDIR)

		// creating the directory for the chaoslib, if not present
		chaoslibDIR := litmusRootDir + "/chaoslib/litmus/" + experimentDetails.Name
		CreateDirectoryIfNotPresent(chaoslibDIR)

		// creating the directory for the environment variables file, if not present
		experimentPKGDirectory := litmusRootDir + "/pkg/" + experimentDetails.Category
		CreateDirectoryIfNotPresent(experimentPKGDirectory)
		experimentPKGSubDirectory := experimentPKGDirectory + "/" + experimentDetails.Name
		CreateDirectoryIfNotPresent(experimentPKGSubDirectory)
		environmentDIR := experimentPKGSubDirectory + "/environment"
		CreateDirectoryIfNotPresent(environmentDIR)

		// creating the directory for the types.go file, if not present
		typesDIR := experimentPKGSubDirectory + "/environment"
		CreateDirectoryIfNotPresent(typesDIR)

		//creating the directory for test deployment
		testDIR := experimentDIR + "/" + "test"
		CreateDirectoryIfNotPresent(testDIR)

		// generating the experiement.go file
		experimentFilePath := experimentDIR + "/" + experimentDetails.Name + ".go"
		GenerateFile(experimentDetails, experimentFilePath, "./templates/experiment.tmpl")

		// generating the csv file
		csvFilePath := experimentDIR + "/" + experimentDetails.Name + ".chartserviceversion.yaml"
		GenerateFile(experimentDetails, csvFilePath, "./templates/chartserviceversion.tmpl")

		// generating the chart file
		chartFilePath := experimentDIR + "/" + "experiment.yaml"
		GenerateFile(experimentDetails, chartFilePath, "./templates/experiment_custom_resource.tmpl")

		// generating the rbac file
		rbacFilePath := experimentDIR + "/" + "rbac.yaml"
		GenerateFile(experimentDetails, rbacFilePath, "./templates/experiment_rbac.tmpl")

		// generating the engine file
		engineFilePath := experimentDIR + "/" + "engine.yaml"
		GenerateFile(experimentDetails, engineFilePath, "./templates/experiment_engine.tmpl")

		// generating the test deployment file
		testDeploymentFilePath := testDIR + "/" + "test.yml"
		GenerateFile(experimentDetails, testDeploymentFilePath, "./templates/experiment_k8s_deployment.tmpl")

		// generating the chaoslib file
		chaoslibFilePath := chaoslibDIR + "/" + experimentDetails.Name + ".go"
		GenerateFile(experimentDetails, chaoslibFilePath, "./templates/chaoslib.tmpl")

		// generating the environment var file
		environmentFilePath := environmentDIR + "/" + "environment.go"
		GenerateFile(experimentDetails, environmentFilePath, "./templates/environment.tmpl")

		// generating the types.go file
		typesFilePath := typesDIR + "/" + "types.go"
		GenerateFile(experimentDetails, typesFilePath, "./templates/types.tmpl")

	}

}

// GetConfig load data from YAML file into a structure
// the object of structure can be further use to bootstrap the provided template for the experiment
func GetConfig(experimentDetails *types.Experiment, attributeFile string) *types.Experiment {

	yamlFile, err := ioutil.ReadFile(attributeFile)
	if err != nil {
		log.Printf("Unable to read the yaml file, due to err: %v", err)
	}
	err = yaml.Unmarshal(yamlFile, experimentDetails)
	if err != nil {
		log.Fatalf("Unable to unmarshal due to err : %v", err)
	}

	return experimentDetails
}

// GenerateFile bootstrap the file from the template
func GenerateFile(experimentDetails types.Experiment, fileName string, templatePath string) {

	// parse the experiment template
	tpl, err := template.ParseFiles(templatePath)
	if err != nil {
		log.Fatalln(err)
	}
	// store the bootstraped file in the buffer
	var out bytes.Buffer
	err = tpl.Execute(&out, experimentDetails)
	if err != nil {
		panic(err)
	}

	// write the date into the destination file
	err = ioutil.WriteFile(fileName, out.Bytes(), 0644)
	check(err)

}

// CreateDirectoryIfNotPresent check for the directory and create if not present
func CreateDirectoryIfNotPresent(path string) {
	_, err := os.Stat(path)

	if os.IsNotExist(err) {
		os.Mkdir(path, 0755)
	}

}

// It will check for the err to be nil
func check(e error) {
	if e != nil {
		panic(e)
	}
}
