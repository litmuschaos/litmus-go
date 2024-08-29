package utils

import "net/url"

const (
	OTELExporterOTLPEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"
)

func HttpTimeout(err error) bool {
	httpErr := err.(*url.Error)
	if httpErr != nil {
		return httpErr.Timeout()
	}
	return false
}
