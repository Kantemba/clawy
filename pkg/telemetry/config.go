// Clawy - Ultra-lightweight personal AI agent
//
// Copyright (c) 2026 Clawy contributors

package telemetry

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// ProtocolGRPC selects OTLP over gRPC.
	ProtocolGRPC = "grpc"
	// ProtocolHTTP selects OTLP over HTTP with protobuf payloads.
	ProtocolHTTP = "http"

	// DefaultServiceName is used when no service name is configured.
	DefaultServiceName = "clawy"
	// DefaultEndpoint is the standard OTLP gRPC collector port.
	DefaultEndpoint = "localhost:4317"
	// DefaultProtocol is the default OTLP transport protocol.
	DefaultProtocol = ProtocolGRPC
	// DefaultSampleRatio keeps every span by default.
	DefaultSampleRatio = 1.0
	// DefaultExportTimeout bounds each exporter attempt.
	DefaultExportTimeout = 10 * time.Second
	// DefaultMetricInterval is the periodic metric reader interval.
	DefaultMetricInterval = 60 * time.Second
)

// Config holds OpenTelemetry SDK settings. Values may come from the Clawy
// config file (JSON), CLAWY_TELEMETRY_* environment variables, or fall back
// to the standard OTEL_* variables defined by the OpenTelemetry specification.
type Config struct {
	Enabled        bool              `json:"enabled,omitempty"         env:"CLAWY_TELEMETRY_ENABLED"`
	ServiceName    string            `json:"service_name,omitempty"    env:"CLAWY_TELEMETRY_SERVICE_NAME"`
	Environment    string            `json:"environment,omitempty"     env:"CLAWY_TELEMETRY_ENVIRONMENT"`
	Endpoint       string            `json:"endpoint,omitempty"        env:"CLAWY_TELEMETRY_ENDPOINT"`
	Protocol       string            `json:"protocol,omitempty"        env:"CLAWY_TELEMETRY_PROTOCOL"`
	Insecure       bool              `json:"insecure,omitempty"        env:"CLAWY_TELEMETRY_INSECURE"`
	// Headers may be set in the config file or via the standard
	// OTEL_EXPORTER_OTLP_HEADERS environment variable.
	Headers        map[string]string `json:"headers,omitempty"`
	SampleRatio    float64           `json:"sample_ratio,omitempty"    env:"CLAWY_TELEMETRY_SAMPLE_RATIO"`
	TracesEnabled  bool              `json:"traces_enabled,omitempty"  env:"CLAWY_TELEMETRY_TRACES_ENABLED"`
	MetricsEnabled bool              `json:"metrics_enabled,omitempty" env:"CLAWY_TELEMETRY_METRICS_ENABLED"`
	LogsEnabled    bool              `json:"logs_enabled,omitempty"    env:"CLAWY_TELEMETRY_LOGS_ENABLED"`
	RuntimeMetrics bool              `json:"runtime_metrics,omitempty" env:"CLAWY_TELEMETRY_RUNTIME_METRICS"`

	ExportTimeout  time.Duration `json:"-" yaml:"-"`
	MetricInterval time.Duration `json:"-" yaml:"-"`
}

// Normalized returns a copy with defaults and OTEL_* env fallbacks applied.
func (c Config) Normalized() Config {
	return c.normalize()
}

func (c Config) normalize() Config {
	if c.ServiceName == "" {
		c.ServiceName = firstNonEmpty(os.Getenv("OTEL_SERVICE_NAME"), DefaultServiceName)
	}
	if c.Environment == "" {
		c.Environment = firstNonEmpty(os.Getenv("OTEL_DEPLOYMENT_ENVIRONMENT"), "production")
	}
	if c.Endpoint == "" {
		c.Endpoint = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
	if c.Endpoint == "" {
		c.Endpoint = DefaultEndpoint
	}
	if c.Protocol == "" {
		c.Protocol = firstNonEmpty(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"), DefaultProtocol)
	}
	switch c.Protocol {
	case ProtocolHTTP, "http/protobuf", "http/json":
		c.Protocol = ProtocolHTTP
	default:
		c.Protocol = ProtocolGRPC
	}

	// Standard OTEL_EXPORTER_OTLP_INSECURE only applies to grpc endpoints.
	if !c.Insecure && c.Protocol == ProtocolGRPC {
		if v := os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"); v != "" {
			c.Insecure, _ = strconv.ParseBool(v)
		}
	}

	if len(c.Headers) == 0 {
		c.Headers = parseHeadersEnv(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"))
	}

	if c.SampleRatio <= 0 || c.SampleRatio > 1 {
		c.SampleRatio = parseSampleRatioEnv(os.Getenv("OTEL_TRACES_SAMPLER_ARG"), DefaultSampleRatio)
	}

	if c.ExportTimeout <= 0 {
		if ms := intEnv("CLAWY_TELEMETRY_EXPORT_TIMEOUT_MS", 0); ms > 0 {
			c.ExportTimeout = time.Duration(ms) * time.Millisecond
		} else if ms := intEnv("OTEL_EXPORTER_OTLP_TIMEOUT", 0); ms > 0 {
			c.ExportTimeout = time.Duration(ms) * time.Millisecond
		} else {
			c.ExportTimeout = DefaultExportTimeout
		}
	}
	if c.MetricInterval <= 0 {
		if s := intEnv("CLAWY_TELEMETRY_METRIC_INTERVAL_S", 0); s > 0 {
			c.MetricInterval = time.Duration(s) * time.Second
		} else if s := intEnv("OTEL_METRIC_EXPORT_INTERVAL", 0); s > 0 {
			c.MetricInterval = time.Duration(s) * time.Second
		} else {
			c.MetricInterval = DefaultMetricInterval
		}
	}

	return c
}

// Validate reports whether the resolved config can drive an OTLP pipeline.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("telemetry endpoint must not be empty")
	}
	switch c.Protocol {
	case ProtocolGRPC, ProtocolHTTP:
	default:
		return fmt.Errorf("unsupported telemetry protocol %q (use %q or %q)", c.Protocol, ProtocolGRPC, ProtocolHTTP)
	}
	return nil
}

// anySignalEnabled reports whether at least one signal will be exported.
func (c Config) anySignalEnabled() bool {
	return c.TracesEnabled || c.MetricsEnabled || c.LogsEnabled
}

func parseHeadersEnv(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	headers := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		if key == "" {
			continue
		}
		decoded, err := strconv.Unquote(`"` + value + `"`)
		if err == nil {
			value = decoded
		}
		headers[key] = value
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func parseSampleRatioEnv(raw string, fallback float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	ratio, err := strconv.ParseFloat(raw, 64)
	if err != nil || ratio <= 0 || ratio > 1 {
		return fallback
	}
	return ratio
}

func intEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
