package cmd

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"github.com/litmuschaos/litmus-go/contribute/developer-guide/types"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// GenerateExperiment generate the new/custom chaos experiment based on specified attribute file
func GenerateExperiment(attributeFile *string, generationType string) error {

	// Fetch all the required attributes from the given file
	// Experiment contains all the required attributes
	var experimentDetails types.Experiment
	if err := GetConfig(&experimentDetails, *attributeFile); err != nil {
		return err
	}

	// getting the current directory name
	currDir, err := os.Getwd()
	if err != nil {
		return err
	}
	// generating the parent directory name
	// so that it can be utilise to get the relative path of all files from there
	currDir = path.Join(currDir, "..")
	litmusRootDir := filepath.Dir(currDir)

	// creating the directory of experiment category, if not present
	chartDIR := litmusRootDir + "/experiments/" + experimentDetails.Category
	CreateDirectoryIfNotPresent(chartDIR)

	if generationType == "chart" {
		csvFilePath := chartDIR + "/" + experimentDetails.Category + ".chartserviceversion.yaml"
		if err = GenerateFile(experimentDetails, csvFilePath, "./templates/category_chartserviceversion.tmpl"); err != nil {
			return err
		}
		packageFilePath := chartDIR + "/" + experimentDetails.Category + ".package.yaml"
		if err = GenerateFile(experimentDetails, packageFilePath, "./templates/package.tmpl"); err != nil {
			return err
		}
	} else if generationType == "experiment" {

		// creating the directory for the experiment, if not present
		experimentRootDIR := chartDIR + "/" + experimentDetails.Name
		CreateDirectoryIfNotPresent(experimentRootDIR)
		experimentDIR := experimentRootDIR + "/experiment"
		CreateDirectoryIfNotPresent(experimentDIR)

		// creating the directory for the chaoslib, if not present
		chaoslibRootDIR := litmusRootDir + "/chaoslib/litmus/" + experimentDetails.Name
		CreateDirectoryIfNotPresent(chaoslibRootDIR)
		chaoslibDIR := chaoslibRootDIR + "/lib"
		CreateDirectoryIfNotPresent(chaoslibDIR)

		// creating the directory for the environment variables file, if not present
		experimentPKGDirectory := litmusRootDir + "/pkg/" + experimentDetails.Category
		CreateDirectoryIfNotPresent(experimentPKGDirectory)
		experimentPKGSubDirectory := experimentPKGDirectory + "/" + experimentDetails.Name
		CreateDirectoryIfNotPresent(experimentPKGSubDirectory)
		environmentDIR := experimentPKGSubDirectory + "/environment"
		CreateDirectoryIfNotPresent(environmentDIR)

		// creating the directory for the types.go file, if not present
		typesDIR := experimentPKGSubDirectory + "/types"
		CreateDirectoryIfNotPresent(typesDIR)

		//creating the directory for test deployment
		testDIR := experimentRootDIR + "/" + "test"
		CreateDirectoryIfNotPresent(testDIR)

		// generating the experiement.go file
		experimentFilePath := experimentDIR + "/" + experimentDetails.Name + ".go"
		if err = GenerateFile(experimentDetails, experimentFilePath, "./templates/experiment.tmpl"); err != nil {
			return err
		}

		// generating the csv file
		csvFilePath := experimentRootDIR + "/" + experimentDetails.Name + ".chartserviceversion.yaml"
		if err = GenerateFile(experimentDetails, csvFilePath, "./templates/experiment_chartserviceversion.tmpl"); err != nil {
			return err
		}

		// generating the chart file
		chartFilePath := experimentRootDIR + "/" + "experiment.yaml"
		GenerateFile(experimentDetails, chartFilePath, "./templates/experiment_custom_resource.tmpl")

		// generating the rbac file
		rbacFilePath := experimentRootDIR + "/" + "rbac.yaml"
		if err = GenerateFile(experimentDetails, rbacFilePath, "./templates/experiment_rbac.tmpl"); err != nil {
			return err
		}

		// generating the engine file
		engineFilePath := experimentRootDIR + "/" + "engine.yaml"
		if err = GenerateFile(experimentDetails, engineFilePath, "./templates/experiment_engine.tmpl"); err != nil {
			return err
		}

		// generating the test deployment file
		testDeploymentFilePath := testDIR + "/" + "test.yml"
		if err = GenerateFile(experimentDetails, testDeploymentFilePath, "./templates/experiment_k8s_deployment.tmpl"); err != nil {
			return err
		}

		// generating the chaoslib file
		chaoslibFilePath := chaoslibDIR + "/" + experimentDetails.Name + ".go"
		if err = GenerateFile(experimentDetails, chaoslibFilePath, "./templates/chaoslib.tmpl"); err != nil {
			return err
		}

		// generating the environment var file
		environmentFilePath := environmentDIR + "/" + "environment.go"
		if err = GenerateFile(experimentDetails, environmentFilePath, "./templates/environment.tmpl"); err != nil {
			return err
		}

		// generating the types.go file
		typesFilePath := typesDIR + "/" + "types.go"
		if err = GenerateFile(experimentDetails, typesFilePath, "./templates/types.tmpl"); err != nil {
			return err
		}

	}
	return nil
}

// GetConfig load data from YAML file into a structure
// the object of structure can be further use to bootstrap the provided template for the experiment
func GetConfig(experimentDetails *types.Experiment, attributeFile string) error {

	yamlFile, err := ioutil.ReadFile(attributeFile)
	if err != nil {
		return errors.Errorf("Unable to read the yaml file, err: %v", err)
	}
	err = yaml.Unmarshal(yamlFile, experimentDetails)
	if err != nil {
		return errors.Errorf("Unable to unmarshal, err: %v", err)
	}

	return nil
}

// GenerateFile bootstrap the file from the template
func GenerateFile(experimentDetails types.Experiment, fileName string, templatePath string) error {

	// parse the experiment template
	tpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return err
	}
	// store the bootstraped file in the buffer
	var out bytes.Buffer
	err = tpl.Execute(&out, experimentDetails)
	if err != nil {
		return err
	}

	// write the date into the destination file
	err = ioutil.WriteFile(fileName, out.Bytes(), 0644)
	return err
}

// CreateDirectoryIfNotPresent check for the directory and create if not present
func CreateDirectoryIfNotPresent(path string) {
	_, err := os.Stat(path)

	if os.IsNotExist(err) {
		os.Mkdir(path, 0755)
	}

}
