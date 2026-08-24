// Clawy - Ultra-lightweight personal AI agent
//
// Copyright (c) 2026 Clawy contributors

package gateway

import (
	"context"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/logger"
	"github.com/Kantemba/clawy/pkg/telemetry"
)

const logSinkScope = "github.com/Kantemba/clawy/pkg/logger"

// stopLogSink detaches the OTLP log bridge on shutdown; set when logs export.
var stopLogSink func()

// initTelemetry boots the OpenTelemetry SDK from the loaded configuration and
// bridges application logs into the OTLP log pipeline. It returns an error
// only when telemetry was explicitly enabled but could not start; a disabled
// configuration returns nil so the gateway runs without instrumentation.
func initTelemetry(cfg *config.Config) error {
	effective := cfg.EffectiveTelemetry()

	instance, err := telemetry.Setup(context.Background(), effective, config.GetVersion())
	if err != nil {
		return err
	}
	if !instance.Enabled() {
		return nil
	}

	if instance.LogsEnabled() {
		sink := telemetry.NewLogRecordWriter(logSinkScope, config.GetVersion())
		logger.AttachSink(sink)
		stopLogSink = func() { logger.DetachSink(sink) }
	}

	return nil
}

// releaseTelemetry detaches the log bridge before providers close.
func releaseTelemetry() {
	if stopLogSink != nil {
		stopLogSink()
		stopLogSink = nil
	}
}
