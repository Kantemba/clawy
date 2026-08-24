package config

import (
	"github.com/Kantemba/clawy/pkg/telemetry"
)

// TelemetryConfig is an alias so the OpenTelemetry settings live alongside
// the rest of the config schema while keeping a single source of truth for
// JSON keys, environment overrides, and normalization.
type TelemetryConfig = telemetry.Config

// EffectiveTelemetry returns the normalized telemetry configuration,
// applying standard OTEL_* environment fallbacks. When telemetry is enabled
// without explicit per-signal flags, all three signals are exported.
func (c *Config) EffectiveTelemetry() TelemetryConfig {
	if c == nil {
		return telemetry.Config{}.Normalized()
	}
	cfg := c.Telemetry
	if cfg.Enabled && !cfg.TracesEnabled && !cfg.MetricsEnabled && !cfg.LogsEnabled {
		cfg.TracesEnabled = true
		cfg.MetricsEnabled = true
		cfg.LogsEnabled = true
	}
	return cfg.Normalized()
}
