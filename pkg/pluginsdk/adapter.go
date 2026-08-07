package pluginsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	toolshared "github.com/Kantemba/clawy/pkg/tools/shared"
)

// Adapter bridges the plugin SDK with the agent's internal systems.
// It implements the Host interface so plugins can access agent services.
type Adapter struct {
	ctx            context.Context
	toolRegistry   ToolRegistryAdapter
	memoryProvider MemoryProvider
	logger         Logger
	configProvider ConfigProvider
	hookBroker     HookBroker
}

// ToolRegistryAdapter wraps the agent's tool registry for plugin tool registration.
type ToolRegistryAdapter interface {
	RegisterTool(def ToolDefinition) error
}

// MemoryProvider gives plugins access to session memory.
type MemoryProvider interface {
	GetMemory(sessionKey string) (MemoryAccess, error)
}

// ConfigProvider returns plugin-specific configuration.
type ConfigProvider interface {
	GetPluginConfig(pluginName string) json.RawMessage
}

// HookBroker allows plugins to subscribe to agent event hooks.
type HookBroker interface {
	Subscribe(hook string, handler HookHandler)
}

// NewAdapter creates a new Host adapter.
func NewAdapter(
	ctx context.Context,
	memoryProvider MemoryProvider,
	configProvider ConfigProvider,
	hookBroker HookBroker,
) *Adapter {
	return &Adapter{
		ctx:            ctx,
		memoryProvider: memoryProvider,
		configProvider: configProvider,
		hookBroker:     hookBroker,
		logger:         &defaultLogger{},
	}
}

// Context returns the agent's base context.
func (a *Adapter) Context() context.Context {
	return a.ctx
}

// Memory returns memory access for a session.
func (a *Adapter) Memory(sessionKey string) MemoryAccess {
	access, err := a.memoryProvider.GetMemory(sessionKey)
	if err != nil {
		slog.Warn("plugin memory access failed", "session", sessionKey, "error", err)
		return nil
	}
	return access
}

// RegisterTool registers a plugin tool with the agent's registry.
func (a *Adapter) RegisterTool(def ToolDefinition) error {
	if a.toolRegistry == nil {
		return fmt.Errorf("tool registry not configured")
	}
	return a.toolRegistry.RegisterTool(def)
}

// SubscribeHook registers a hook handler.
func (a *Adapter) SubscribeHook(hook string, handler HookHandler) error {
	if a.hookBroker == nil {
		return fmt.Errorf("hook broker not configured")
	}
	a.hookBroker.Subscribe(hook, handler)
	return nil
}

// Logger returns the adapter's logger.
func (a *Adapter) Logger() Logger {
	return a.logger
}

// Config returns plugin-specific configuration.
func (a *Adapter) Config() json.RawMessage {
	return nil
}

// SetToolRegistry sets the tool registry adapter.
func (a *Adapter) SetToolRegistry(reg ToolRegistryAdapter) {
	a.toolRegistry = reg
}

// PluginToTool converts a plugin ToolDefinition to a toolshared.Tool adapter.
func PluginToTool(def ToolDefinition) toolshared.Tool {
	return &pluginToolAdapter{def: def}
}

// pluginToolAdapter adapts a plugin ToolDefinition to the toolshared.Tool interface.
type pluginToolAdapter struct {
	def ToolDefinition
}

func (t *pluginToolAdapter) Name() string                        { return t.def.Name }
func (t *pluginToolAdapter) Description() string                 { return t.def.Description }
func (t *pluginToolAdapter) Parameters() map[string]any          { return t.def.Parameters }
func (t *pluginToolAdapter) Execute(ctx context.Context, args map[string]any) *toolshared.ToolResult {
	result := t.def.Execute(ctx, args)
	if result == nil {
		return toolshared.ErrorResult("plugin tool returned nil result")
	}
	return &toolshared.ToolResult{
		ForLLM:  result.ContentForLLM,
		ForUser: result.ContentForUser,
		IsError: result.IsError,
		Async:   result.Async,
	}
}

// defaultLogger is a simple slog-backed logger.
type defaultLogger struct{}

func (l *defaultLogger) Debug(msg string, args ...any) { slog.Debug(msg, args...) }
func (l *defaultLogger) Info(msg string, args ...any)  { slog.Info(msg, args...) }
func (l *defaultLogger) Warn(msg string, args ...any)  { slog.Warn(msg, args...) }
func (l *defaultLogger) Error(msg string, args ...any) { slog.Error(msg, args...) }
