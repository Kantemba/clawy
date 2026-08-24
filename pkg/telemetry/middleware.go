// Clawy - Ultra-lightweight personal AI agent
//
// Copyright (c) 2026 Clawy contributors

package telemetry

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

// HTTPHandler wraps an http.Handler with OpenTelemetry tracing and metrics.
// When telemetry is disabled the handler is returned unchanged so the
// middleware adds zero overhead.
func HTTPHandler(handler http.Handler, operation string) http.Handler {
	instance := Get()
	if !instance.Enabled() {
		return handler
	}

	opts := []otelhttp.Option{
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
	}
	if instance.MetricsEnabled() {
		opts = append(opts, otelhttp.WithMeterProvider(instance.meterProvider))
	}
	if !instance.TracesEnabled() {
		opts = append(opts, otelhttp.WithSpanNameFormatter(func(_ string, _ *http.Request) string {
			return operation
		}))
	}

	return otelhttp.NewHandler(handler, operation, opts...)
}

// HTTPClient wraps an *http.Client transport with OTel propagation and spans
// for outbound requests. Returns the client unchanged when disabled.
func HTTPClient(client *http.Client, operation string) *http.Client {
	if client == nil {
		return nil
	}
	if !Get().Enabled() {
		return client
	}

	wrapped := &http.Client{
		Transport: otelhttp.NewTransport(
			client.Transport,
			otelhttp.WithPropagators(otel.GetTextMapPropagator()),
			otelhttp.WithSpanNameFormatter(func(_ string, _ *http.Request) string { return operation }),
		),
		CheckRedirect: client.CheckRedirect,
		Jar:           client.Jar,
		Timeout:       client.Timeout,
	}
	return wrapped
}
