// Clawy - Ultra-lightweight personal AI agent
//
// Copyright (c) 2026 Clawy contributors

package telemetry

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kantemba/clawy/pkg/logger"
)

func resetGlobals(t *testing.T) {
	t.Helper()
	globalState.Store(nil)
	t.Cleanup(func() {
		globalState.Store(nil)
	})
	os.Unsetenv("OTEL_SERVICE_NAME")
	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	os.Unsetenv("OTEL_EXPORTER_OTLP_PROTOCOL")
}

func TestSetupDisabled(t *testing.T) {
	resetGlobals(t)

	instance, err := Setup(context.Background(), Config{}, "test")
	if err != nil {
		t.Fatalf("Setup(disabled) returned error: %v", err)
	}
	if instance.Enabled() {
		t.Fatal("instance should not be enabled when config is empty")
	}
	if err := Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown on disabled telemetry returned error: %v", err)
	}
}

func TestNormalizeEnvFallbacks(t *testing.T) {
	resetGlobals(t)

	t.Setenv("OTEL_SERVICE_NAME", "svc-from-env")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "collector.example.com:4318")

	cfg := Config{Enabled: true}.Normalized()
	if cfg.ServiceName != "svc-from-env" {
		t.Fatalf("service name = %q, want svc-from-env", cfg.ServiceName)
	}
	if cfg.Endpoint != "collector.example.com:4318" {
		t.Fatalf("endpoint = %q, want collector.example.com:4318", cfg.Endpoint)
	}
	if cfg.Protocol != ProtocolGRPC {
		t.Fatalf("protocol = %q, want %q", cfg.Protocol, ProtocolGRPC)
	}
	if cfg.SampleRatio != DefaultSampleRatio {
		t.Fatalf("sample ratio = %v, want %v", cfg.SampleRatio, DefaultSampleRatio)
	}
}

func TestNormalizeSchemeEndpoint(t *testing.T) {
	resetGlobals(t)

	cfg := Config{
		Enabled:  true,
		Endpoint: "https://collector.example.com",
	}.Normalized()

	host, insecure := endpointOptions(cfg)
	if host[0] != "collector.example.com" {
		t.Fatalf("host = %q, want collector.example.com", host[0])
	}
	if insecure {
		t.Fatal("https endpoint must not be insecure")
	}

	cfg = Config{
		Enabled:  true,
		Endpoint: "http://127.0.0.1:4318",
		Protocol: ProtocolHTTP,
	}.Normalized()
	_, insecure = endpointOptions(cfg)
	if !insecure {
		t.Fatal("http endpoint must be insecure")
	}
}

func TestValidate(t *testing.T) {
	resetGlobals(t)

	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("disabled config should always validate: %v", err)
	}
	if err := (Config{Enabled: true}).Validate(); err == nil {
		t.Fatal("enabled config without endpoint should fail validation")
	}
	if err := (Config{
		Enabled:  true,
		Endpoint: "localhost:4317",
		Protocol: "carrier-pigeon",
	}).Validate(); err == nil {
		t.Fatal("unsupported protocol should fail validation")
	}
}

// TestLogBridgeEndToEnd spins up a fake OTLP/HTTP collector, wires the
// zerolog bridge, emits one record, and verifies the payload reaches it.
func TestLogBridgeEndToEnd(t *testing.T) {
	resetGlobals(t)

	var mu sync.Mutex
	var received [][]byte
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/logs" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mu.Lock()
		received = append(received, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()

	host := strings.TrimPrefix(collector.URL, "http://")
	instance, err := Setup(context.Background(), Config{
		Enabled:       true,
		ServiceName:   "clawy-test",
		Endpoint:      host,
		Protocol:      ProtocolHTTP,
		LogsEnabled:   true,
		ExportTimeout: 5 * time.Second,
	}, "test")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if !instance.LogsEnabled() {
		t.Fatal("logs signal should be enabled")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = Shutdown(ctx)
	}()

	sink := NewLogRecordWriter(logTestScope, "test")
	logger.AttachSink(sink)
	defer logger.DetachSink(sink)

	logger.InfoCF("telemetry", "bridge smoke test", map[string]any{"channel": "unit"})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(received)
		payload := append([]byte(nil), bytes.Join(received, nil)...)
		mu.Unlock()
		if got > 0 && bytes.Contains(payload, []byte("bridge smoke test")) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("log record never reached the collector")
}

const logTestScope = "github.com/Kantemba/clawy/pkg/telemetry.test"
