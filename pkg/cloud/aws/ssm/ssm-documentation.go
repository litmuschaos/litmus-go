package ssm

import (
	"io/ioutil"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	"github.com/pkg/errors"
)

// CreateAndUploadDocument will create and add the ssm document in aws service monitoring docs.
func CreateAndUploadDocument(documentName, documentType, documentFormat, documentPath, region string) error {

	sesh := common.GetAWSSession(region)
	openFile, err := ioutil.ReadFile(documentPath)
	if err != nil {
		return errors.Errorf("fail to read the file err: %v", err)
	}
	documentContent := string(openFile)

	// Load session from shared config
	ssmClient := ssm.New(sesh)
	_, err = ssmClient.CreateDocument(&ssm.CreateDocumentInput{
		Content:        &documentContent,
		Name:           aws.String(documentName),
		DocumentType:   aws.String(documentType),
		DocumentFormat: aws.String(documentFormat),
		VersionName:    aws.String("1"),
	})

	if err != nil {
		return errors.Errorf("fail to create docs, err: %v", err)
	}
	return nil
}

// SSMDeleteDocument will delete all the versions of docs uploaded for the chaos.
func SSMDeleteDocument(documentName, region string) error {

	sesh := common.GetAWSSession(region)
	ssmClient := ssm.New(sesh)
	_, err := ssmClient.DeleteDocument(&ssm.DeleteDocumentInput{
		Name: aws.String(documentName),
	})
	if err != nil {
		return common.CheckAWSError(err)
	}
	return nil
}
