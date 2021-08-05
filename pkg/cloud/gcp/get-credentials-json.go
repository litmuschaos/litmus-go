package gcp

import (
	"encoding/json"
	"io/ioutil"
	"strings"
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
	fileContentByteSlice, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	fileContentString := string(fileContentByteSlice)

	if fileContentString[len(fileContentString)-1] == '\n' {
		fileContentString = fileContentString[:len(fileContentString)-1]
	}

	return fileContentString, nil
}

// GetServiceAccountJSONFromSecret fetches the secrets mounted as volume and returns the json credentials byte slice
func GetServiceAccountJSONFromSecret() ([]byte, error) {
	gcpType, err := getFileContent("/tmp/type")
	if err != nil {
		return []byte{}, err
	}

	gcpProjectID, err := getFileContent("/tmp/project_id")
	if err != nil {
		return []byte{}, err
	}

	gcpPrivateKeyID, err := getFileContent("/tmp/private_key_id")
	if err != nil {
		return []byte{}, err
	}

	gcpPrivateKey, err := getFileContent("/tmp/private_key")
	if err != nil {
		return []byte{}, err
	}

	gcpClientEmail, err := getFileContent("/tmp/client_email")
	if err != nil {
		return []byte{}, err
	}

	gcpClientID, err := getFileContent("/tmp/client_id")
	if err != nil {
		return []byte{}, err
	}

	gcpAuthURI, err := getFileContent("/tmp/auth_uri")
	if err != nil {
		return []byte{}, err
	}

	gcpTokenURI, err := getFileContent("/tmp/token_uri")
	if err != nil {
		return []byte{}, err
	}

	gcpAuthCertURL, err := getFileContent("/tmp/auth_provider_x509_cert_url")
	if err != nil {
		return []byte{}, err
	}

	gcpClientCertURL, err := getFileContent("/tmp/client_x509_cert_url")
	if err != nil {
		return []byte{}, err
	}

	credentials := GCPServiceAccountCredentials{gcpType, gcpProjectID, gcpPrivateKeyID, gcpPrivateKey, gcpClientEmail, gcpClientID, gcpAuthURI, gcpTokenURI, gcpAuthCertURL, gcpClientCertURL}
	credentials.GCPPrivateKey = strings.Replace(credentials.GCPPrivateKey, "\\n", "\n", -1)

	byteSliceJSONString, err := json.Marshal(credentials)
	if err != nil {
		return []byte{}, err
	}

	return byteSliceJSONString, nil
}
