// Clawy - Ultra-lightweight personal AI agent
//
// Copyright (c) 2026 Clawy contributors

// Package telemetry wires the OpenTelemetry SDK for traces, metrics, and logs
// and exports them via OTLP (gRPC or HTTP/protobuf). When disabled, every
// helper degrades to a no-op so callers never need nil checks.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Kantemba/clawy/pkg/logger"
)

const (
	instrumentationName    = "github.com/Kantemba/clawy"
	instrumentationVersion = "1.0.0"

	defaultLogBatchDelay = 5 * time.Second
	shutdownGracePeriod  = 10 * time.Second
)

var (
	globalState atomic.Pointer[Telemetry]
	setupMu     sync.Mutex
)

// Telemetry owns the OpenTelemetry providers for a running gateway.
type Telemetry struct {
	cfg Config

	tracerProvider  *sdktrace.TracerProvider
	meterProvider   *sdkmetric.MeterProvider
	loggerProvider  *sdklog.LoggerProvider
	runtimeStopOnce sync.Once
}

// Config returns the normalized configuration in use.
func (t *Telemetry) Config() Config {
	if t == nil {
		return Config{}
	}
	return t.cfg
}

// Enabled reports whether any OTLP signal is being exported.
func (t *Telemetry) Enabled() bool {
	return t != nil && t.cfg.Enabled && t.cfg.anySignalEnabled()
}

// TracesEnabled reports whether spans are exported.
func (t *Telemetry) TracesEnabled() bool { return t != nil && t.cfg.Enabled && t.cfg.TracesEnabled }

// MetricsEnabled reports whether metrics are exported.
func (t *Telemetry) MetricsEnabled() bool { return t != nil && t.cfg.Enabled && t.cfg.MetricsEnabled }

// LogsEnabled reports whether logs are exported.
func (t *Telemetry) LogsEnabled() bool { return t != nil && t.cfg.Enabled && t.cfg.LogsEnabled }

// Setup initializes the global OpenTelemetry SDK. The first call wins; later
// calls return the existing instance unchanged. A disabled config installs
// nothing and returns a no-op instance.
func Setup(ctx context.Context, cfg Config, version string) (*Telemetry, error) {
	setupMu.Lock()
	defer setupMu.Unlock()

	if existing := globalState.Load(); existing != nil {
		return existing, nil
	}

	cfg = cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid telemetry config: %w", err)
	}

	instance := &Telemetry{cfg: cfg}
	if cfg.Enabled && cfg.anySignalEnabled() {
		if err := instance.start(ctx, version); err != nil {
			return nil, err
		}
		logger.InfoCF("telemetry", "OpenTelemetry enabled", map[string]any{
			"endpoint":        cfg.Endpoint,
			"protocol":        cfg.Protocol,
			"service_name":    cfg.ServiceName,
			"environment":     cfg.Environment,
			"sample_ratio":    cfg.SampleRatio,
			"traces":          cfg.TracesEnabled,
			"metrics":         cfg.MetricsEnabled,
			"logs":            cfg.LogsEnabled,
			"runtime_metrics": cfg.RuntimeMetrics,
		})
	}
	globalState.Store(instance)
	return instance, nil
}

// Get returns the previously configured telemetry instance (nil if Setup was
// never called).
func Get() *Telemetry { return globalState.Load() }

// Enabled reports whether telemetry is active globally.
func Enabled() bool { return Get().Enabled() }

// Shutdown flushes and releases all providers. Call once at process exit.
func Shutdown(ctx context.Context) error {
	instance := globalState.Load()
	if instance == nil || !instance.Enabled() {
		return nil
	}
	return instance.shutdown(ctx)
}

func (t *Telemetry) start(ctx context.Context, version string) error {
	cfg := t.cfg

	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(propagator)

	resource, err := buildResource(cfg, version)
	if err != nil {
		return fmt.Errorf("build otel resource: %w", err)
	}

	if cfg.TracesEnabled {
		exporter, err := newTraceExporter(ctx, cfg)
		if err != nil {
			return fmt.Errorf("create trace exporter: %w", err)
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(resource),
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
			sdktrace.WithBatcher(exporter),
		)
		otel.SetTracerProvider(tp)
		t.tracerProvider = tp
	}

	if cfg.MetricsEnabled {
		exporter, err := newMetricExporter(ctx, cfg)
		if err != nil {
			return fmt.Errorf("create metric exporter: %w", err)
		}
		reader := sdkmetric.NewPeriodicReader(
			exporter,
			sdkmetric.WithInterval(cfg.MetricInterval),
		)
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(resource),
			sdkmetric.WithReader(reader),
		)
		otel.SetMeterProvider(mp)
		t.meterProvider = mp
		if err = initMeters(mp); err != nil {
			return fmt.Errorf("create instruments: %w", err)
		}

		if cfg.RuntimeMetrics {
			if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
				return fmt.Errorf("start runtime instrumentation: %w", err)
			}
		}
	}

	if cfg.LogsEnabled {
		exporter, err := newLogExporter(ctx, cfg)
		if err != nil {
			return fmt.Errorf("create log exporter: %w", err)
		}
		lp := sdklog.NewLoggerProvider(
			sdklog.WithResource(resource),
			sdklog.WithProcessor(sdklog.NewBatchProcessor(
				exporter,
				sdklog.WithExportInterval(defaultLogBatchDelay),
			)),
		)
		global.SetLoggerProvider(lp)
		t.loggerProvider = lp
	}

	return nil
}

func (t *Telemetry) shutdown(ctx context.Context) error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), shutdownGracePeriod)
		defer cancel()
	}

	var errs []error

	if t.loggerProvider != nil {
		if err := t.loggerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown log provider: %w", err))
		}
	}
	if t.meterProvider != nil {
		if err := t.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown meter provider: %w", err))
		}
	}
	if t.tracerProvider != nil {
		if err := t.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown tracer provider: %w", err))
		}
	}
	t.runtimeStopOnce.Do(func() {})

	return errors.Join(errs...)
}

func buildResource(cfg Config, version string) (*resource.Resource, error) {
	attrs := []attributePair{
		attributeString("service.name", cfg.ServiceName),
		attributeString("deployment.environment.name", cfg.Environment),
	}
	if version != "" {
		attrs = append(attrs, attributeString("service.version", version))
	}
	if host, err := os.Hostname(); err == nil && host != "" {
		attrs = append(attrs, attributeString("host.name", host))
	}

	return newResource(attrs...)
}

func endpointOptions(cfg Config) ([]string, bool) {
	raw := strings.TrimSpace(cfg.Endpoint)
	insecure := cfg.Insecure

	// Allow scheme-style endpoints (http:// or https://) for both transports.
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		switch u.Scheme {
		case "http":
			insecure = true
		case "https":
			insecure = false
		default:
			return []string{raw}, insecure
		}
		return []string{u.Host}, insecure
	}

	// Bare host:port with the HTTP transport means plaintext, matching the
	// standard insecure OTLP/HTTP port 4318. Explicit https:// opts into TLS.
	if cfg.Protocol == ProtocolHTTP && raw != "" && !strings.Contains(raw, "://") {
		insecure = true
	}
	return []string{raw}, insecure
}
