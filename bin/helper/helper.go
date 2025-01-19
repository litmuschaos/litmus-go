package main

import (
	"context"
	"errors"
	"flag"
	"os"

	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"

	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/openstack"

	containerKill "github.com/litmuschaos/litmus-go/chaoslib/litmus/container-kill/helper"
	diskFill "github.com/litmuschaos/litmus-go/chaoslib/litmus/disk-fill/helper"
	httpChaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/helper"
	networkChaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/helper"
	dnsChaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/pod-dns-chaos/helper"
	stressChaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/stress-chaos/helper"
	cli "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

func init() {
	// Log as JSON instead of the default ASCII formatter
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:          true,
		DisableSorting:         true,
		DisableLevelTruncation: true,
	})
}

func main() {
	ctx := context.Background()
	// Set up Observability.
	if otelExporterEndpoint := os.Getenv(telemetry.OTELExporterOTLPEndpoint); otelExporterEndpoint != "" {
		shutdown, err := telemetry.InitOTelSDK(ctx, true, otelExporterEndpoint)
		if err != nil {
			log.Errorf("Failed to initialize OTel SDK: %v", err)
			return
		}
		defer func() {
			err = errors.Join(err, shutdown(ctx))
		}()
		ctx = telemetry.GetTraceParentContext()
	}

	clients := cli.ClientSets{}

	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "ExecuteExperimentHelper")
	defer span.End()

	// parse the helper name
	helperName := flag.String("name", "", "name of the helper pod")

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Errorf("Unable to Get the kubeconfig, err: %v", err)
		return
	}

	log.Infof("Helper Name: %v", *helperName)

	// invoke the corresponding helper based on the the (-name) flag
	switch *helperName {
	case "container-kill":
		containerKill.Helper(ctx, clients)
	case "disk-fill":
		diskFill.Helper(ctx, clients)
	case "dns-chaos":
		dnsChaos.Helper(ctx, clients)
	case "stress-chaos":
		stressChaos.Helper(ctx, clients)
	case "network-chaos":
		networkChaos.Helper(ctx, clients)
	case "http-chaos":
		httpChaos.Helper(ctx, clients)

	default:
		log.Errorf("Unsupported -name %v, please provide the correct value of -name args", *helperName)
		return
	}
}
