// Package mcpserver implements native Model Context Protocol (MCP) server
// support for Clawy. It exposes Clawy's tools to external MCP clients (such as
// Claude Desktop, Cursor, or any other MCP-compatible client) so that they can
// discover and invoke Clawy's capabilities through the standard MCP protocol.
//
// The server supports three transports:
//   - stdio: communication over stdin/stdout (newline-delimited JSON)
//   - sse: Server-Sent Events with a dual endpoint (POST for requests, GET for SSE stream)
//   - http: Streamable HTTP (MCP 2025-03-26 spec), request-response mode
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Kantemba/clawy/pkg/config"
	runtimeevents "github.com/Kantemba/clawy/pkg/events"
	"github.com/Kantemba/clawy/pkg/logger"
	toolshared "github.com/Kantemba/clawy/pkg/tools/shared"
)

const (
	// ServerName identifies the Clawy MCP server instance.
	ServerName = "clawy"

	// ServerVersion is advertised to MCP clients during initialization.
	ServerVersion = "1.0.0"

	// ServerInstructions is the optional human-readable guidance shown to MCP
	// clients during initialization.
	ServerInstructions = "Clawy native MCP server. Exposes Clawy's tools to MCP clients."

	// ShutdownTimeout bounds how long the server waits for in-flight HTTP
	// connections to drain before forcing close.
	ShutdownTimeout = 15 * time.Second

	// DefaultToolExecutionTimeout bounds how long a single tool call via the
	// native MCP server may run before it is aborted.
	DefaultToolExecutionTimeout = 30 * time.Second
)

// ToolProvider returns the list of tools available from the server.
// This is typically backed by a tools.ToolRegistry or a filtered subset.
type ToolProvider func() []toolshared.Tool

// Server is a native MCP server that exposes Clawy tools to external clients.
type Server struct {
	impl         *mcp.Implementation
	server       *mcp.Server
	cfg          config.NativeServerConfig
	instructions string
	toolProvider ToolProvider
	eventBus     runtimeevents.Bus
	execTimeout  time.Duration

	mu        sync.Mutex
	transport string
	httpSrv   *http.Server
	sessions  []*mcp.ServerSession
	ctx       context.Context
	cancel    context.CancelFunc
	started   bool
	closed    bool
}

// NewServer creates a new native MCP server from configuration.
func NewServer(
	cfg config.NativeServerConfig,
	toolProvider ToolProvider,
	opts ...Option,
) *Server {
	s := &Server{
		impl: &mcp.Implementation{
			Name:    ServerName,
			Version: ServerVersion,
			Title:   "Clawy",
		},
		cfg:          cfg,
		instructions: ServerInstructions,
		toolProvider: toolProvider,
		execTimeout:  DefaultToolExecutionTimeout,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}

	return s
}

// Option configures a Server.
type Option func(*Server)

// WithEventBus injects the runtime event bus used for MCP server lifecycle
// events.
func WithEventBus(bus runtimeevents.Bus) Option {
	return func(s *Server) {
		s.eventBus = bus
	}
}

// WithInstructions overrides the server instructions shown to clients.
func WithInstructions(instructions string) Option {
	return func(s *Server) {
		if instructions != "" {
			s.instructions = instructions
		}
	}
}

// WithToolExecutionTimeout overrides the default tool execution timeout.
func WithToolExecutionTimeout(d time.Duration) Option {
	return func(s *Server) {
		if d > 0 {
			s.execTimeout = d
		}
	}
}

// buildToolHandler creates an MCP ToolHandler that delegates to an internal
// Clawy tool, injecting the "mcp" channel scope into the context so tools
// that read channel/chatID via context still function.
func buildToolHandler(tool toolshared.Tool, execTimeout time.Duration) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		if req.Params != nil && len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{
							Text: fmt.Sprintf("Invalid arguments: %s", err),
						},
					},
					IsError: true,
				}, nil
			}
		}

		// Inject MCP channel context so tools that rely on ToolChannel/ToolChatID
		// receive a consistent value.
		ctx = toolshared.WithToolContext(ctx, "mcp", req.Params.Name)

		// Apply an execution timeout so a stuck tool does not block the MCP
		// session indefinitely.
		timeoutCtx, cancel := context.WithTimeout(ctx, execTimeout)
		defer cancel()

		result := tool.Execute(timeoutCtx, args)
		if result == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool returned no result"},
				},
				IsError: true,
			}, nil
		}

		return toMCPResult(result), nil
	}
}

// toMCPResult converts an internal ToolResult to an MCP CallToolResult.
func toMCPResult(result *toolshared.ToolResult) *mcp.CallToolResult {
	content := []mcp.Content{
		&mcp.TextContent{Text: result.ContentForLLM()},
	}

	return &mcp.CallToolResult{
		Content: content,
		IsError: result.IsError,
	}
}

// registerTools registers all tools from the ToolProvider onto the MCP server.
func (s *Server) registerTools() {
	if s.toolProvider == nil {
		return
	}

	tools := s.toolProvider()
	for _, tool := range tools {
		if tool == nil {
			continue
		}

		mcpTool := &mcp.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.Parameters(),
		}

		handler := buildToolHandler(tool, s.execTimeout)
		s.server.AddTool(mcpTool, handler)

		logger.DebugCF("mcpserver", "Registered tool on native MCP server",
			map[string]any{
				"tool": tool.Name(),
			})
	}

	s.publishEvent(runtimeevents.KindMCPServerStarted,
		fmt.Sprintf("Registered %d tools", len(tools)), nil)
}

// publishEvent emits a lifecycle event if the event bus is configured.
func (s *Server) publishEvent(kind runtimeevents.Kind, message string, err error) {
	if s.eventBus == nil {
		return
	}

	severity := runtimeevents.SeverityInfo
	if err != nil {
		severity = runtimeevents.SeverityError
	}

	payload := NativeServerEventPayload{
		Transport: s.transport,
		Host:      s.cfg.Host,
		Port:      s.cfg.Port,
		Path:      s.cfg.Path,
		Message:   message,
	}
	if err != nil {
		payload.Error = err.Error()
	}

	s.eventBus.PublishNonBlocking(runtimeevents.Event{
		Kind:     kind,
		Source:   runtimeevents.Source{Component: "mcp_server"},
		Severity: severity,
		Payload:  payload,
		Attrs: map[string]any{
			"transport": s.transport,
			"host":      s.cfg.Host,
			"port":      s.cfg.Port,
			"path":      s.cfg.Path,
		},
	})
}

// NativeServerEventPayload describes native MCP server lifecycle events.
type NativeServerEventPayload struct {
	Transport string `json:"transport"`
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	Path      string `json:"path,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Start begins serving MCP clients. It is non-blocking for all transports
// (stdio, sse, http); the serving goroutine(s) are managed internally and
// will stop when ctx is canceled or Close() is called.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("native MCP server is already started")
	}
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("native MCP server is closed")
	}

	s.transport = config.EffectiveNativeTransport(s.cfg)

	s.server = mcp.NewServer(s.impl, &mcp.ServerOptions{
		Instructions: s.instructions,
	})

	s.registerTools()

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.started = true
	s.mu.Unlock()

	switch s.transport {
	case "stdio":
		go s.serveStdio(s.ctx)
	case "sse":
		go s.serveSSE(s.ctx)
	case "http":
		go s.serveHTTP(s.ctx)
	default:
		_ = s.Close()
		return fmt.Errorf(
			"unsupported native MCP transport: %s (supported: stdio, sse, http)",
			s.transport,
		)
	}

	return nil
}

// serveStdio runs the MCP server on the stdio transport.
func (s *Server) serveStdio(ctx context.Context) {
	transport := &mcp.StdioTransport{}

	if err := s.server.Run(ctx, transport); err != nil {
		s.publishEvent(runtimeevents.KindMCPServerFailed, "", err)
		logger.ErrorCF("mcpserver", "Native MCP server (stdio) stopped with error",
			map[string]any{"error": err.Error()})
	}

	s.publishEvent(runtimeevents.KindMCPServerStopped, "", nil)
}

// serveSSE starts the SSE-based MCP server.
func (s *Server) serveSSE(ctx context.Context) error {
	handler := mcp.NewSSEHandler(func(*http.Request) *mcp.Server {
		return s.server
	}, &mcp.SSEOptions{
		DisableLocalhostProtection: s.cfg.Host == "0.0.0.0" || s.cfg.Host == "",
	})

	mux := http.NewServeMux()
	path := config.EffectiveNativePath(s.cfg)
	mux.Handle(path, handler)

	addr := s.bindAddress()

	s.httpSrv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	s.publishEvent(runtimeevents.KindMCPServerStarted,
		fmt.Sprintf("Listening on http://%s%s", addr, path), nil)

	logger.InfoCF("mcpserver", "Native MCP server (SSE) listening",
		map[string]any{
			"addr": addr,
			"path": path,
		})

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.publishEvent(runtimeevents.KindMCPServerFailed, "", err)
			logger.ErrorCF("mcpserver", "Native MCP server (SSE) failed",
				map[string]any{"error": err.Error()})
		}
	}()

	<-ctx.Done()
	s.publishEvent(runtimeevents.KindMCPServerStopped, "", nil)
	return nil
}

// serveHTTP starts the streamable HTTP MCP server.
func (s *Server) serveHTTP(ctx context.Context) error {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.server
	}, &mcp.StreamableHTTPOptions{
		DisableLocalhostProtection: s.cfg.Host == "0.0.0.0" || s.cfg.Host == "",
	})

	mux := http.NewServeMux()
	path := config.EffectiveNativePath(s.cfg)
	mux.Handle(path, handler)

	addr := s.bindAddress()

	s.httpSrv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	s.publishEvent(runtimeevents.KindMCPServerStarted,
		fmt.Sprintf("Listening on http://%s%s", addr, path), nil)

	logger.InfoCF("mcpserver", "Native MCP server (HTTP) listening",
		map[string]any{
			"addr": addr,
			"path": path,
		})

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.publishEvent(runtimeevents.KindMCPServerFailed, "", err)
			logger.ErrorCF("mcpserver", "Native MCP server (HTTP) failed",
				map[string]any{"error": err.Error()})
		}
	}()

	<-ctx.Done()
	s.publishEvent(runtimeevents.KindMCPServerStopped, "", nil)
	return nil
}

// bindAddress computes the TCP bind address for SSE/HTTP transports.
func (s *Server) bindAddress() string {
	host := config.EffectiveNativeHost(s.cfg)
	port := config.EffectiveNativePort(s.cfg)
	return fmt.Sprintf("%s:%d", host, port)
}

// Close gracefully shuts down the MCP server and releases resources.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true

	// Cancel the internal context to unblock serve goroutines.
	if s.cancel != nil {
		s.cancel()
	}

	var errs []error

	// Capture httpSrv before releasing the lock so Shutdown (which may block)
	// does not hold the mutex.
	httpSrv := s.httpSrv
	implServer := s.server
	s.mu.Unlock()

	if httpSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("HTTP server shutdown: %w", err))
		}
	}

	if implServer != nil {
		for _, session := range s.sessions {
			if err := session.Close(); err != nil {
				errs = append(errs, fmt.Errorf("session close: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("native MCP server close errors: %w", errs[0])
	}

	return nil
}

// Config returns the native server configuration.
func (s *Server) Config() config.NativeServerConfig {
	return s.cfg
}

// Transport returns the active transport type.
func (s *Server) Transport() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transport
}

// IsStarted reports whether the server has been started.
func (s *Server) IsStarted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}
