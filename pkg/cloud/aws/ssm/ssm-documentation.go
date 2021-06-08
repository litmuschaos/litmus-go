package ssm

import (
	"fmt"
	"io/ioutil"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

// CreateAndUploadDocument will create and add the ssm document in aws service monitoring docs.
func CreateAndUploadDocument(doccumentName, doccumentType, doccumentFormat, doccumentPath, region string) error {

	sesh := common.GetAWSSession(region)
	openFile, err := ioutil.ReadFile(doccumentPath)
	if err != nil {
		return errors.Errorf("fail to read the file err: %v", err)
	}
	documentContent := string(openFile)

	// Load session from shared config
	ssmClient := ssm.New(sesh)
	_, err = ssmClient.CreateDocument(&ssm.CreateDocumentInput{
		Content:        &documentContent,
		Name:           aws.String(doccumentName),
		DocumentType:   aws.String(doccumentType),
		DocumentFormat: aws.String(doccumentFormat),
		VersionName:    aws.String("1"),
	})

	if err != nil {
		fmt.Println("vc qucbqi")
		return errors.Errorf("err: %v", err)
	}
	return nil
}

// SSMDeleteDocument will delete all the versions of docs uploaded for the chaos.
func SSMDeleteDocument(doccumentName, region string) error {

	sesh := common.GetAWSSession(region)
	ssmClient := ssm.New(sesh)
	_, err := ssmClient.DeleteDocument(&ssm.DeleteDocumentInput{
		Name: aws.String(doccumentName),
	})
	if err != nil {
		return common.CheckAWSError(err)
	}
	return nil
}
