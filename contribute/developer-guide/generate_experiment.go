package cmd

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/litmuschaos/litmus-go/contribute/developer-guide/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// GenerateExperiment generate the new/custom chaos experiment based on specified attribute file
func GenerateExperiment(attributeFile, chartType string, generationType string) error {

	// Fetch all the required attributes from the given file
	// Experiment contains all the required attributes
	var experimentDetails types.Experiment
	if err := getConfig(&experimentDetails, attributeFile); err != nil {
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
	categoryDIR := litmusRootDir + "/experiments/" + experimentDetails.Category
	createDirectoryIfNotPresent(categoryDIR)

	// creating the directory for the experiment, if not present
	experimentRootDIR := categoryDIR + "/" + experimentDetails.Name
	createDirectoryIfNotPresent(experimentRootDIR)

	switch strings.ToLower(generationType) {
	case "chart":

		if chartType == "category" || chartType == "all" {
			// create category charts
			if err := createCategoryCharts(litmusRootDir, experimentRootDIR, experimentDetails); err != nil {
				return err
			}
		}

		if chartType == "experiment" || chartType == "all" {
			// create experiment charts
			if err := createExperimentCharts(litmusRootDir, experimentRootDIR, experimentDetails); err != nil {
				return err
			}
		}

	default:

		// creating experiment dir & files
		if err := createExperiment(experimentRootDIR, experimentDetails); err != nil {
			return err
		}

		// creating chaoslib dir & files
		if err := createChaosLib(litmusRootDir, experimentDetails); err != nil {
			return err
		}

		// creating envs dir & files
		if err := createENV(litmusRootDir, experimentDetails); err != nil {
			return err
		}

		// creating test dir & files
		if err := createOkteto(experimentRootDIR, experimentDetails); err != nil {
			return err
		}
	}

	return nil
}

// getConfig load data from YAML file into a structure
// the object of structure can be further use to bootstrap the provided template for the experiment
func getConfig(experimentDetails *types.Experiment, attributeFile string) error {

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

// generateFile bootstrap the file from the template
func generateFile(experimentDetails types.Experiment, fileName string, templatePath string) error {

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

// createDirectoryIfNotPresent check for the directory and create if not present
func createDirectoryIfNotPresent(path string) {
	_, err := os.Stat(path)

	if os.IsNotExist(err) {
		os.Mkdir(path, 0755)
	}
}

// copy creates a copy of the source file at the destination
func copy(src, dest string) error {
	command := exec.Command("cp", src, dest)
	var out, stderr bytes.Buffer
	command.Stdout = &out
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		log.Infof("Error String: %v", stderr.String())
		return errors.Errorf("unable to copy file, err: %v", err)
	}
	return nil
}

// createExperimentFile creates the experiment file
func createExperiment(experimentRootDIR string, experimentDetails types.Experiment) error {
	// create the experiment directory, if not present
	experimentDIR := experimentRootDIR + "/experiment"
	createDirectoryIfNotPresent(experimentDIR)

	// generating the experiement.go file
	experimentFilePath := experimentDIR + "/" + experimentDetails.Name + ".go"
	return generateFile(experimentDetails, experimentFilePath, "./templates/experiment.tmpl")
}

// createChaosLib creates the chaoslib for the experiment
func createChaosLib(litmusRootDir string, experimentDetails types.Experiment) error {
	// create the chaoslib directory, if not present
	chaoslibRootDIR := litmusRootDir + "/chaoslib/litmus/" + experimentDetails.Name
	createDirectoryIfNotPresent(chaoslibRootDIR)
	chaoslibDIR := chaoslibRootDIR + "/lib"
	createDirectoryIfNotPresent(chaoslibDIR)

	// generating the chaoslib file
	chaoslibFilePath := chaoslibDIR + "/" + experimentDetails.Name + ".go"
	return generateFile(experimentDetails, chaoslibFilePath, "./templates/chaoslib.tmpl")
}

// createENV creates the env getter and setter files
func createENV(litmusRootDir string, experimentDetails types.Experiment) error {
	// creating the directory for the environment variables file, if not present
	experimentPKGDirectory := litmusRootDir + "/pkg/" + experimentDetails.Category
	createDirectoryIfNotPresent(experimentPKGDirectory)
	experimentPKGSubDirectory := experimentPKGDirectory + "/" + experimentDetails.Name
	createDirectoryIfNotPresent(experimentPKGSubDirectory)

	// creating the directory for the environment.go file, if not present
	environmentDIR := experimentPKGSubDirectory + "/environment"
	createDirectoryIfNotPresent(environmentDIR)

	// creating the directory for the types.go file, if not present
	typesDIR := experimentPKGSubDirectory + "/types"
	createDirectoryIfNotPresent(typesDIR)

	// generating the environment var file
	environmentFilePath := environmentDIR + "/" + "environment.go"
	if err := generateFile(experimentDetails, environmentFilePath, "./templates/environment.tmpl"); err != nil {
		return err
	}

	// generating the types.go file
	typesFilePath := typesDIR + "/" + "types.go"
	if err := generateFile(experimentDetails, typesFilePath, "./templates/types.tmpl"); err != nil {
		return err
	}
	return nil
}

// createOkteto creates the test file for okteto
func createOkteto(experimentRootDIR string, experimentDetails types.Experiment) error {
	//creating the directory for test deployment
	testDIR := experimentRootDIR + "/" + "test"
	createDirectoryIfNotPresent(testDIR)

	// generating the test deployment file
	testDeploymentFilePath := testDIR + "/" + "test.yml"
	return generateFile(experimentDetails, testDeploymentFilePath, "./templates/experiment_k8s_deployment.tmpl")
}

// createCategoryCharts creates the category charts
func createCategoryCharts(litmusRootDir, experimentRootDIR string, experimentDetails types.Experiment) error {
	// creating the chart directory, if not present
	chartDirectory := experimentRootDIR + "/charts"
	createDirectoryIfNotPresent(chartDirectory)

	// create parent charts
	csvFilePath := chartDirectory + "/" + experimentDetails.Category + ".chartserviceversion.yaml"
	if err := generateFile(experimentDetails, csvFilePath, "./templates/category_chartserviceversion.tmpl"); err != nil {
		return err
	}
	packageFilePath := chartDirectory + "/" + experimentDetails.Category + ".package.yaml"
	if err := generateFile(experimentDetails, packageFilePath, "./templates/package.tmpl"); err != nil {
		return err
	}

	// creating the icons directory, if not present
	iconDirectory := chartDirectory + "/icons/"
	createDirectoryIfNotPresent(iconDirectory)

	// adding icon for category
	if err := copy(litmusRootDir+types.IconPath, iconDirectory+experimentDetails.Category+".png"); err != nil {
		return err
	}
	return nil
}

// createExperimentCharts creates the experiment charts
func createExperimentCharts(litmusRootDir, experimentRootDIR string, experimentDetails types.Experiment) error {
	// creating the chart directory, if not present
	chartDirectory := experimentRootDIR + "/charts"
	createDirectoryIfNotPresent(chartDirectory)

	// generating the csv file
	csvFilePath := chartDirectory + "/" + experimentDetails.Name + ".chartserviceversion.yaml"
	if err := generateFile(experimentDetails, csvFilePath, "./templates/experiment_chartserviceversion.tmpl"); err != nil {
		return err
	}

	// generating the chart file
	chartFilePath := chartDirectory + "/" + "experiment.yaml"
	generateFile(experimentDetails, chartFilePath, "./templates/experiment_custom_resource.tmpl")

	// generating the rbac file
	rbacFilePath := chartDirectory + "/" + "rbac.yaml"
	if err := generateFile(experimentDetails, rbacFilePath, "./templates/experiment_rbac.tmpl"); err != nil {
		return err
	}

	// generating the engine file
	engineFilePath := chartDirectory + "/" + "engine.yaml"
	if err := generateFile(experimentDetails, engineFilePath, "./templates/experiment_engine.tmpl"); err != nil {
		return err
	}

	// creating the icons directory, if not present
	iconDirectory := chartDirectory + "/icons/"
	createDirectoryIfNotPresent(iconDirectory)
	// adding icon for experiment
	if err := copy(litmusRootDir+types.IconPath, iconDirectory+experimentDetails.Name+".png"); err != nil {
		return err
	}

	return nil
}
