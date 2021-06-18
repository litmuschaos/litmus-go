package gcp

import (
	"fmt"
	"io/ioutil"
)

func GetServiceAccountJSONFromSecret() ([]byte, error) {
	gcpType, err := ioutil.ReadFile("/tmp/type")
	if err != nil {
		return []byte{}, err
	}
	gcpProjectID, err := ioutil.ReadFile("/tmp/project_id")
	if err != nil {
		return []byte{}, err
	}
	gcpPrivateKeyID, err := ioutil.ReadFile("/tmp/private_key_id")
	if err != nil {
		return []byte{}, err
	}
	gcpPrivateKey, err := ioutil.ReadFile("/tmp/private_key")
	if err != nil {
		return []byte{}, err
	}
	gcpClientEmail, err := ioutil.ReadFile("/tmp/client_email")
	if err != nil {
		return []byte{}, err
	}
	gcpClientID, err := ioutil.ReadFile("/tmp/client_id")
	if err != nil {
		return []byte{}, err
	}
	gcpAuthURI, err := ioutil.ReadFile("/tmp/auth_uri")
	if err != nil {
		return []byte{}, err
	}
	gcpTokenURI, err := ioutil.ReadFile("/tmp/token_uri")
	if err != nil {
		return []byte{}, err
	}
	gcpAuthCertURL, err := ioutil.ReadFile("/tmp/auth_provider_x509_cert_url")
	if err != nil {
		return []byte{}, err
	}
	gcpClientCertURL, err := ioutil.ReadFile("/tmp/client_x509_cert_url")
	if err != nil {
		return []byte{}, err
	}

	jsonString := fmt.Sprintf(`{
		"type": "%s",
		"project_id": "%s",
		"private_key_id": "%s",
		"private_key": "%s",
		"client_email": "%s",
		"client_id": "%s",
		"auth_uri": "%s",
		"token_uri": "%s",
		"auth_provider_x509_cert_url": "%s",
		"client_x509_cert_url": "%s"
		}`, string(gcpType)[:len(string(gcpType))-1],
		string(gcpProjectID)[:len(string(gcpProjectID))-1],
		string(gcpPrivateKeyID)[:len(string(gcpPrivateKeyID))-1],
		string(gcpPrivateKey)[:len(string(gcpPrivateKey))-1],
		string(gcpClientEmail)[:len(string(gcpClientEmail))-1],
		string(gcpClientID)[:len(string(gcpClientID))-1],
		string(gcpAuthURI)[:len(string(gcpAuthURI))-1],
		string(gcpTokenURI)[:len(string(gcpTokenURI))-1],
		string(gcpAuthCertURL)[:len(string(gcpAuthCertURL))-1],
		string(gcpClientCertURL)[:len(string(gcpClientCertURL))-1],
	)

	return []byte(jsonString), nil
}
