// Clawy - Ultra-lightweight personal AI agent
//
// Copyright (c) 2026 Clawy contributors

package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"
)

const (
	zerologFieldLevel   = "level"
	zerologFieldMessage = "message"
	zerologFieldTraceID = "trace_id"
	zerologFieldSpanID  = "span_id"
)

// LogRecordWriter is an io.Writer that consumes zerolog JSON lines and emits
// them as OpenTelemetry log records through the global LoggerProvider.
// Attach it with logger.AttachSink so application logs flow into OTLP.
type LogRecordWriter struct {
	mu      sync.Mutex
	buf     []byte
	emitter otellog.Logger
}

var _ io.Writer = (*LogRecordWriter)(nil)

// NewLogRecordWriter builds a sink that emits records under the given
// instrumentation scope.
func NewLogRecordWriter(name, version string) *LogRecordWriter {
	provider := global.GetLoggerProvider()
	return &LogRecordWriter{
		emitter: provider.Logger(name, otellog.WithInstrumentationVersion(version)),
	}
}

func (w *LogRecordWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n := len(p)
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := w.buf[:idx]
		w.buf = w.buf[idx+1:]
		if len(line) == 0 {
			continue
		}
		w.emit(line)
	}
	// Guard against pathological unbounded growth when a caller writes very
	// long lines without a trailing newline.
	const maxBuffered = 1 << 20
	if len(w.buf) > maxBuffered {
		w.emit(w.buf)
		w.buf = w.buf[:0]
	}
	return n, nil
}

func (w *LogRecordWriter) emit(line []byte) {
	var fields map[string]any
	if err := json.Unmarshal(line, &fields); err != nil {
		// Not structured zerolog output; forward the raw text as the body.
		fields = map[string]any{zerologFieldMessage: strings.TrimSpace(string(line))}
	}

	record := otellog.Record{}
	record.SetTimestamp(time.Now())
	record.SetSeverity(mapSeverity(fields))
	record.SetBody(otellog.StringValue(messageText(fields)))

	attrs := make([]otellog.KeyValue, 0, 8)
	var traceID trace.TraceID
	var spanID trace.SpanID
	for key, value := range fields {
		switch key {
		case zerologFieldLevel, zerologFieldMessage:
			continue
		case zerologFieldTraceID:
			if id, ok := value.(string); ok && id != "" {
				traceID, _ = trace.TraceIDFromHex(id)
			}
			continue
		case zerologFieldSpanID:
			if id, ok := value.(string); ok && id != "" {
				spanID, _ = trace.SpanIDFromHex(id)
			}
			continue
		}
		if kv, ok := toKeyValue(key, value); ok {
			attrs = append(attrs, kv)
		}
	}
	if len(attrs) > 0 {
		record.AddAttributes(attrs...)
	}

	ctx := context.Background()
	if traceID.IsValid() && spanID.IsValid() {
		ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
			TraceID: traceID,
			SpanID:  spanID,
		}))
	}
	w.emitter.Emit(ctx, record)
}

func messageText(fields map[string]any) string {
	if msg, ok := fields[zerologFieldMessage].(string); ok && msg != "" {
		return msg
	}
	return ""
}

func mapSeverity(fields map[string]any) otellog.Severity {
	level, _ := fields[zerologFieldLevel].(string)
	switch strings.ToLower(level) {
	case "trace", "debug":
		return otellog.SeverityDebug
	case "", "info":
		return otellog.SeverityInfo
	case "warn", "warning":
		return otellog.SeverityWarn
	case "error":
		return otellog.SeverityError
	case "fatal", "panic":
		return otellog.SeverityFatal
	default:
		return otellog.SeverityInfo
	}
}

func toKeyValue(key string, value any) (otellog.KeyValue, bool) {
	switch v := value.(type) {
	case nil:
		return otellog.String(key, "null"), true
	case string:
		return otellog.String(key, v), true
	case bool:
		return otellog.Bool(key, v), true
	case float64:
		if v == float64(int64(v)) {
			return otellog.Int64(key, int64(v)), true
		}
		return otellog.Float64(key, v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return otellog.Int64(key, i), true
		}
		f, err := v.Float64()
		if err != nil {
			return otellog.KeyValue{}, false
		}
		return otellog.Float64(key, f), true
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return otellog.KeyValue{}, false
		}
		return otellog.String(key, string(raw)), true
	}
}
