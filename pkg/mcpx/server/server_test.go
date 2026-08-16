package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Kantemba/clawy/pkg/config"
	toolshared "github.com/Kantemba/clawy/pkg/tools/shared"
)

type mockTool struct {
	name        string
	description string
	params      map[string]any
	result      *toolshared.ToolResult
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return t.description }
func (t *mockTool) Parameters() map[string]any {
	if t.params == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return t.params
}
func (t *mockTool) Execute(ctx context.Context, args map[string]any) *toolshared.ToolResult {
	return t.result
}

func nativeConfig(transport string) config.NativeServerConfig {
	return config.NativeServerConfig{
		Enabled:   true,
		Transport: transport,
		Host:      "127.0.0.1",
		Port:      0,
		Path:      "/mcp",
	}
}

func TestNewServer_InitialState(t *testing.T) {
	srv := NewServer(nativeConfig("stdio"), func() []toolshared.Tool { return nil })

	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.IsStarted() {
		t.Error("server should not be started after construction")
	}
	if srv.Transport() != "" {
		t.Errorf("transport should be empty before Start, got %q", srv.Transport())
	}
}

func TestStart_StdioRegistersToolHandler(t *testing.T) {
	mt := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		params:      map[string]any{"type": "object", "properties": map[string]any{}},
		result:      toolshared.NewToolResult("ok"),
	}
	srv := NewServer(nativeConfig("stdio"), func() []toolshared.Tool { return []toolshared.Tool{mt} })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if transport := srv.Transport(); transport != "stdio" {
		t.Errorf("expected transport 'stdio', got %q", transport)
	}
	if err := srv.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestStart_DoubleStartReturnsError(t *testing.T) {
	srv := NewServer(nativeConfig("stdio"), func() []toolshared.Tool { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	if err := srv.Start(ctx); err == nil {
		t.Error("second Start should return error")
	}
	_ = srv.Close()
}

func TestStart_UnsupportedTransportReturnsError(t *testing.T) {
	srv := NewServer(nativeConfig("invalid"), func() []toolshared.Tool { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err == nil {
		t.Fatal("expected error for unsupported transport")
	}
}

func TestToMCPResult_TextContent(t *testing.T) {
	result := toolshared.NewToolResult("hello world")
	mcpResult := toMCPResult(result)

	if len(mcpResult.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(mcpResult.Content))
	}
	if mcpResult.IsError {
		t.Error("expected IsError=false for non-error result")
	}

	text, ok := mcpResult.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", mcpResult.Content[0])
	}
	if text.Text != "hello world" {
		t.Errorf("expected 'hello world', got %q", text.Text)
	}
}

func TestToMCPResult_ErrorFlag(t *testing.T) {
	result := toolshared.ErrorResult("something went wrong")
	mcpResult := toMCPResult(result)

	if !mcpResult.IsError {
		t.Error("expected IsError=true for error result")
	}

	text, ok := mcpResult.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", mcpResult.Content[0])
	}
	if text.Text != "something went wrong" {
		t.Errorf("expected error message, got %q", text.Text)
	}
}

func TestBuildToolHandler_InvalidArguments(t *testing.T) {
	mt := &mockTool{
		name:   "bad_args",
		params: map[string]any{"type": "object", "properties": map[string]any{}},
		result: toolshared.NewToolResult("should not reach"),
	}

	handler := buildToolHandler(mt, 30*time.Second)

	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{
		Arguments: json.RawMessage(`{invalid json}`),
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for invalid arguments")
	}
}

func TestBuildToolHandler_SuccessfulExecution(t *testing.T) {
	mt := &mockTool{
		name:   "good_tool",
		params: map[string]any{"type": "object", "properties": map[string]any{}},
		result: toolshared.NewToolResult("success!"),
	}

	handler := buildToolHandler(mt, 30*time.Second)

	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{
		Arguments: json.RawMessage(`{"key":"value"}`),
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for successful execution")
	}

	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if text.Text != "success!" {
		t.Errorf("expected 'success!', got %q", text.Text)
	}
}

func TestBuildToolHandler_NilResult(t *testing.T) {
	mt := &mockTool{
		name:   "nil_tool",
		params: map[string]any{"type": "object", "properties": map[string]any{}},
		result: nil,
	}

	handler := buildToolHandler(mt, 30*time.Second)

	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for nil result")
	}
}

func TestRegisterTools_NilProvider(t *testing.T) {
	srv := NewServer(nativeConfig("stdio"), nil)

	srv.mu.Lock()
	defer srv.mu.Unlock()

	srv.registerTools()
}

func TestServer_Options(t *testing.T) {
	provider := func() []toolshared.Tool { return nil }

	t.Run("WithInstructions", func(t *testing.T) {
		srv := NewServer(nativeConfig("stdio"), provider, WithInstructions("Custom instructions"))
		if srv.instructions != "Custom instructions" {
			t.Errorf("expected custom instructions, got %q", srv.instructions)
		}
	})

	t.Run("WithInstructionsEmpty", func(t *testing.T) {
		srv := NewServer(nativeConfig("stdio"), provider, WithInstructions(""))
		if srv.instructions != ServerInstructions {
			t.Errorf("expected default instructions, got %q", srv.instructions)
		}
	})

	t.Run("WithToolExecutionTimeout", func(t *testing.T) {
		srv := NewServer(nativeConfig("stdio"), provider, WithToolExecutionTimeout(60*time.Second))
		if srv.execTimeout != 60*time.Second {
			t.Errorf("expected 60s, got %v", srv.execTimeout)
		}
	})

	t.Run("WithToolExecutionTimeoutZero", func(t *testing.T) {
		srv := NewServer(nativeConfig("stdio"), provider, WithToolExecutionTimeout(0))
		if srv.execTimeout != DefaultToolExecutionTimeout {
			t.Errorf("expected default timeout, got %v", srv.execTimeout)
		}
	})
}

func TestClose_AlreadyClosedIsIdempotent(t *testing.T) {
	srv := NewServer(nativeConfig("stdio"), func() []toolshared.Tool { return nil })

	if err := srv.Close(); err != nil {
		t.Errorf("first Close failed: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Errorf("second Close should be idempotent, got: %v", err)
	}
}

func TestConfigMethod(t *testing.T) {
	cfg := nativeConfig("http")
	cfg.Host = "127.0.0.1"
	cfg.Port = 9999
	cfg.Path = "/custom"

	srv := NewServer(cfg, func() []toolshared.Tool { return nil })

	got := srv.Config()
	if got.Transport != "http" {
		t.Errorf("expected transport 'http', got %q", got.Transport)
	}
	if got.Host != "127.0.0.1" {
		t.Errorf("expected host '127.0.0.1', got %q", got.Host)
	}
	if got.Port != 9999 {
		t.Errorf("expected port 9999, got %d", got.Port)
	}
	if got.Path != "/custom" {
		t.Errorf("expected path '/custom', got %q", got.Path)
	}
}

func TestNativeServerEventPayload(t *testing.T) {
	payload := NativeServerEventPayload{
		Transport: "http",
		Host:      "0.0.0.0",
		Port:      8080,
		Path:      "/mcp",
		Message:   "started",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	expected := `{"transport":"http","host":"0.0.0.0","port":8080,"path":"/mcp","message":"started"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestStart_HTTPServerStarts(t *testing.T) {
	srv := NewServer(nativeConfig("http"), func() []toolshared.Tool { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give the goroutine time to bind.
	time.Sleep(200 * time.Millisecond)

	if transport := srv.Transport(); transport != "http" {
		t.Errorf("expected transport 'http', got %q", transport)
	}

	if err := srv.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestStart_SSEServerStarts(t *testing.T) {
	srv := NewServer(nativeConfig("sse"), func() []toolshared.Tool { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give the goroutine time to bind.
	time.Sleep(200 * time.Millisecond)

	if transport := srv.Transport(); transport != "sse" {
		t.Errorf("expected transport 'sse', got %q", transport)
	}

	if err := srv.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
