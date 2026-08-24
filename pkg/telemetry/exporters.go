// Clawy - Ultra-lightweight personal AI agent
//
// Copyright (c) 2026 Clawy contributors

package telemetry

import (
	"context"
	"time"

	"google.golang.org/grpc/credentials"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

const grpcReconnectPeriod = 5 * time.Second

type attributePair struct {
	key   string
	value string
}

func attributeString(key, value string) attributePair {
	return attributePair{key: key, value: value}
}

// newResource builds an SDK resource with the standard SDK defaults plus the
// supplied service attributes.
func newResource(extra ...attributePair) (*resource.Resource, error) {
	attrs := make([]attribute.KeyValue, 0, len(extra)+2)
	attrs = append(attrs,
		semconv.ServiceName(DefaultServiceName),
		semconv.TelemetrySDKLanguageGo,
	)
	for _, pair := range extra {
		if pair.key == "" || pair.value == "" {
			continue
		}
		attrs = append(attrs, attribute.String(pair.key, pair.value))
	}
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
}

func newTraceExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	host, insecure := endpointOptions(cfg)
	if cfg.Protocol == ProtocolHTTP {
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(host[0]),
			otlptracehttp.WithURLPath("/v1/traces"),
			otlptracehttp.WithHeaders(cfg.Headers),
			otlptracehttp.WithTimeout(cfg.ExportTimeout),
		}
		if insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(ctx, opts...)
	}

	return otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(host[0]),
		otlptracegrpc.WithReconnectionPeriod(grpcReconnectPeriod),
		otlptracegrpc.WithHeaders(cfg.Headers),
		otlptracegrpc.WithTimeout(cfg.ExportTimeout),
		insecureGRPCOption(insecure),
	)
}

func newMetricExporter(ctx context.Context, cfg Config) (sdkmetric.Exporter, error) {
	host, insecure := endpointOptions(cfg)
	if cfg.Protocol == ProtocolHTTP {
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithEndpoint(host[0]),
			otlpmetrichttp.WithURLPath("/v1/metrics"),
			otlpmetrichttp.WithHeaders(cfg.Headers),
			otlpmetrichttp.WithTimeout(cfg.ExportTimeout),
		}
		if insecure {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		return otlpmetrichttp.New(ctx, opts...)
	}

	return otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(host[0]),
		otlpmetricgrpc.WithHeaders(cfg.Headers),
		otlpmetricgrpc.WithTimeout(cfg.ExportTimeout),
		insecureGRPCMetricOption(insecure),
	)
}

func newLogExporter(ctx context.Context, cfg Config) (sdklog.Exporter, error) {
	host, insecure := endpointOptions(cfg)
	if cfg.Protocol == ProtocolHTTP {
		opts := []otlploghttp.Option{
			otlploghttp.WithEndpoint(host[0]),
			otlploghttp.WithURLPath("/v1/logs"),
			otlploghttp.WithHeaders(cfg.Headers),
			otlploghttp.WithTimeout(cfg.ExportTimeout),
		}
		if insecure {
			opts = append(opts, otlploghttp.WithInsecure())
		}
		return otlploghttp.New(ctx, opts...)
	}

	return otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(host[0]),
		otlploggrpc.WithHeaders(cfg.Headers),
		otlploggrpc.WithTimeout(cfg.ExportTimeout),
		insecureGRPCLogOption(insecure),
	)
}

func insecureGRPCOption(insecure bool) otlptracegrpc.Option {
	if insecure {
		return otlptracegrpc.WithInsecure()
	}
	return otlptracegrpc.WithTLSCredentials(credentialsNewTLS())
}

func insecureGRPCMetricOption(insecure bool) otlpmetricgrpc.Option {
	if insecure {
		return otlpmetricgrpc.WithInsecure()
	}
	return otlpmetricgrpc.WithTLSCredentials(credentialsNewTLS())
}

func insecureGRPCLogOption(insecure bool) otlploggrpc.Option {
	if insecure {
		return otlploggrpc.WithInsecure()
	}
	return otlploggrpc.WithTLSCredentials(credentialsNewTLS())
}

func credentialsNewTLS() credentials.TransportCredentials {
	return credentials.NewClientTLSFromCert(nil, "")
}
