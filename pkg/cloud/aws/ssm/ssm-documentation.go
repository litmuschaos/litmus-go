package ssm

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
)

// CreateAndUploadDocument will create and add the ssm document in aws service monitoring docs.
func CreateAndUploadDocument(documentName, documentType, documentFormat, documentPath, region string) error {

	sesh := common.GetAWSSession(region)
	openFile, err := os.ReadFile(documentPath)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to read the file: %v", err),
			Target:    fmt.Sprintf("{SSM Document Path: %v/%v.%v, Region: %v}", documentPath, documentName, documentFormat, region),
		}
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
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to upload docs: %v", err),
			Target:    fmt.Sprintf("{SSM Document Path: %v/%v.%v, Region: %v}", documentPath, documentName, documentFormat, region),
		}
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
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    fmt.Sprintf("failed to delete SSM document: %v", common.CheckAWSError(err).Error()),
			Target:    fmt.Sprintf("{SSM Document Name: %v, Region: %v}", documentName, region),
		}
	}
	return nil
}
