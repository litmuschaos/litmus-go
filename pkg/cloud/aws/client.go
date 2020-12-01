package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/az-down/types"
)

func GetNewEC2Client(experimentsDetails *experimentTypes.ExperimentDetails) (ec2.EC2) {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(experimentsDetails.AwsRegion)},
	}))

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	// TODO add error handling
	return *ec2Svc
}
