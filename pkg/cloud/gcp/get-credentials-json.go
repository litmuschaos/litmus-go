package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

// GCPServiceAccountCredentials stores the service account credentials
type GCPServiceAccountCredentials struct {
	GCPType          string `json:"type"`
	GCPProjectID     string `json:"project_id"`
	GCPPrivateKeyID  string `json:"private_key_id"`
	GCPPrivateKey    string `json:"private_key"`
	GCPClientEmail   string `json:"client_email"`
	GCPClientID      string `json:"client_id"`
	GCPAuthURI       string `json:"auth_uri"`
	GCPTokenURI      string `json:"token_uri"`
	GCPAuthCertURL   string `json:"auth_provider_x509_cert_url"`
	GCPClientCertURL string `json:"client_x509_cert_url"`
}

// getFileContent reads the file content at the given file path
func getFileContent(filePath string) (string, error) {

	fileContentByteSlice, err := os.ReadFile(filePath)
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("failed to read file %s, %s", filePath, err.Error())}
	}

	fileContentString := string(fileContentByteSlice)

	if fileContentString[len(fileContentString)-1] == '\n' {
		fileContentString = fileContentString[:len(fileContentString)-1]
	}

	return fileContentString, nil
}

// getServiceAccountJSONFromSecret fetches the secrets mounted as volume and returns the json credentials byte slice
func getServiceAccountJSONFromSecret() ([]byte, error) {

	gcpType, err := getFileContent("/tmp/type")
	if err != nil {
		return nil, err
	}

	gcpProjectID, err := getFileContent("/tmp/project_id")
	if err != nil {
		return nil, err
	}

	gcpPrivateKeyID, err := getFileContent("/tmp/private_key_id")
	if err != nil {
		return nil, err
	}

	gcpPrivateKey, err := getFileContent("/tmp/private_key")
	if err != nil {
		return nil, err
	}

	gcpClientEmail, err := getFileContent("/tmp/client_email")
	if err != nil {
		return nil, err
	}

	gcpClientID, err := getFileContent("/tmp/client_id")
	if err != nil {
		return nil, err
	}

	gcpAuthURI, err := getFileContent("/tmp/auth_uri")
	if err != nil {
		return nil, err
	}

	gcpTokenURI, err := getFileContent("/tmp/token_uri")
	if err != nil {
		return nil, err
	}

	gcpAuthCertURL, err := getFileContent("/tmp/auth_provider_x509_cert_url")
	if err != nil {
		return nil, err
	}

	gcpClientCertURL, err := getFileContent("/tmp/client_x509_cert_url")
	if err != nil {
		return nil, err
	}

	credentials := GCPServiceAccountCredentials{gcpType, gcpProjectID, gcpPrivateKeyID, gcpPrivateKey, gcpClientEmail, gcpClientID, gcpAuthURI, gcpTokenURI, gcpAuthCertURL, gcpClientCertURL}
	credentials.GCPPrivateKey = strings.Replace(credentials.GCPPrivateKey, "\\n", "\n", -1)

	byteSliceJSONString, err := json.Marshal(credentials)
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("failed to marshal the credentials into json, %s", err.Error())}
	}

	return byteSliceJSONString, nil
}

// doesFileExist checks if a file exists or not
func doesFileExist(fileName string) bool {

	_, err := os.Stat(fileName)

	return err == nil
}

// GetGCPComputeService returns a new compute service created using the GCP Service Account credentials
func GetGCPComputeService() (*compute.Service, error) {

	// create an empty context
	ctx := context.Background()

	for _, fileName := range []string{"type", "project_id", "private_key_id", "private_key", "client_email", "client_id", "auth_uri", "token_uri", "auth_provider_x509_cert_url", "client_x509_cert_url"} {

		if doesFileExist("/tmp/" + fileName) {

			log.Info("[Info]: Using the GCP Service Account credentials from the secret")

			// get service account credentials json
			json, err := getServiceAccountJSONFromSecret()
			if err != nil {
				return nil, err
			}

			// create a new GCP Compute Service client using the GCP service account credentials provided through the secret
			computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
			if err != nil {
				return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("failed to authenticate a new compute service using the given credentials, %s", err.Error())}
			}

			return computeService, nil
		}
	}

	log.Info("[Info]: Using the default GCP Service Account credentials from Worflow Identity")

	// create a new GCP Compute Service client using default GCP service account credentials (using Workload Identity)
	computeService, err := compute.NewService(ctx)
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("failed to authenticate a new compute service using gke workload identity, %s", err.Error())}
	}

	return computeService, nil
}
