package common

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
)

//GetAWSSession will return the aws session for a given region
func GetAWSSession(region string) *session.Session {
	return session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(region)},
	}))
}

//CheckAWSError will return the aws errors
func CheckAWSError(err error) error {
	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		default:
			return errors.Errorf(aerr.Error())
		}
	} else {
		return errors.Errorf(err.Error())
	}
}
