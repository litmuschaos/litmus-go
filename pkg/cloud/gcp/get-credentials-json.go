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
		}`, string(gcpType), string(gcpProjectID), string(gcpPrivateKeyID), string(gcpPrivateKey),
		string(gcpClientEmail), string(gcpClientID), string(gcpAuthURI), string(gcpTokenURI),
		string(gcpAuthCertURL), string(gcpClientCertURL))

	return []byte(jsonString), nil
}
