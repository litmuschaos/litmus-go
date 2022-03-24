package common

import (
	"context"

	"github.com/litmuschaos/litmus-go/pkg/cloud/gcp"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

// GetGCPComputeService returns a new compute service created using the GCP Service Account credentials
func GetGCPComputeService() (*compute.Service, error) {

	// create an empty context
	ctx := context.Background()

	// get service account credentials json
	json, err := gcp.GetServiceAccountJSONFromSecret()
	if err != nil {
		return nil, err
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return nil, err
	}

	return computeService, nil
}
