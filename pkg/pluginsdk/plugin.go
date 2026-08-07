package pluginsdk

import (
	"context"
	"encoding/json"
	"fmt"
)

// Plugin is the core interface that all plugins must implement.
//
// Plugins extend agent capabilities through a typed, code-level interface
// (as opposed to the markdown-based skills system).
type Plugin interface {
	// Metadata returns the plugin's identity and capabilities.
	Metadata() PluginMeta

	// Init is called once when the plugin is loaded. The provided
	// Host gives the plugin access to agent services (memory, tools, etc.).
	// Returns an error if initialization fails.
	Init(host Host) error

	// Shutdown is called when the plugin is being unloaded.
	// Plugins should release resources, stop goroutines, etc.
	Shutdown() error
}

// PluginMeta describes a plugin's identity and requirements.
type PluginMeta struct {
	// Name is the unique plugin identifier (e.g., "weather").
	Name string `json:"name"`

	// Version is the plugin version (semver recommended).
	Version string `json:"version"`

	// Description explains what the plugin does.
	Description string `json:"description"`

	// Author is the plugin creator.
	Author string `json:"author"`

	// APIs lists the agent APIs this plugin depends on.
	// Used to detect compatibility before loading.
	APIs []string `json:"apis,omitempty"`

	// Hooks lists the event hooks this plugin registers for.
	// Valid values: "message:received", "message:sent", "tool:before", "tool:after"
	Hooks []string `json:"hooks,omitempty"`
}

// ToolPlugin is a plugin that provides one or more tools.
type ToolPlugin interface {
	Plugin

	// Tools returns the tool definitions this plugin provides.
	Tools() []ToolDefinition
}

// ToolFunc executes a plugin tool with the given arguments.
type ToolFunc func(ctx context.Context, args map[string]any) *ToolResult

// ToolDefinition describes a tool that a plugin provides.
type ToolDefinition struct {
	// Name is the unique tool name (e.g., "get_weather").
	Name string `json:"name"`

	// Description explains what the tool does for the LLM.
	Description string `json:"description"`

	// Parameters is the JSON Schema for the tool parameters.
	Parameters map[string]any `json:"parameters"`

	// Execute runs the tool with the given arguments.
	Execute ToolFunc
}

// ToolResult is the result of executing a plugin tool.
type ToolResult struct {
	// ContentForLLM is the result text sent to the LLM.
	ContentForLLM string `json:"content_for_llm"`

	// ContentForUser is the result text shown to the user.
	ContentForUser string `json:"content_for_user"`

	// IsError indicates the tool failed.
	IsError bool `json:"is_error"`

	// Async indicates the result will be delivered later.
	Async bool `json:"async"`
}

// NewResult creates a simple success result.
func NewResult(content string) *ToolResult {
	return &ToolResult{
		ContentForLLM:  content,
		ContentForUser: content,
	}
}

// NewErrorResult creates an error result.
func NewErrorResult(err string) *ToolResult {
	return &ToolResult{
		ContentForLLM:  err,
		ContentForUser: err,
		IsError:        true,
	}
}

// Host provides plugins access to agent services.
type Host interface {
	// Context returns the agent's base context.
	Context() context.Context

	// Memory returns the memory store interface for the given session.
	Memory(sessionKey string) MemoryAccess

	// RegisterTool registers a tool provided by this plugin.
	RegisterTool(def ToolDefinition) error

	// SubscribeHook registers a handler for the given event hook.
	SubscribeHook(hook string, handler HookHandler) error

	// Logger returns a structured logger for the plugin.
	Logger() Logger

	// Config returns the plugin-specific configuration as raw JSON.
	Config() json.RawMessage
}

// MemoryAccess provides read-only memory operations for plugins.
type MemoryAccess interface {
	// GetHistory retrieves conversation history.
	GetHistory() ([]Message, error)

	// GetSummary returns the session summary.
	GetSummary() (string, error)

	// Search performs full-text search on session content.
	Search(query string, limit int) ([]SearchMatch, error)
}

// Message is a simplified chat message for plugins.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SearchMatch is a memory search result.
type SearchMatch struct {
	Content   string  `json:"content"`
	Role      string  `json:"role,omitempty"`
	Score     float64 `json:"score"`
	Timestamp int64   `json:"timestamp,omitempty"`
}

// HookHandler processes a hook event.
type HookHandler func(event HookEvent) error

// HookEvent carries data for a hook invocation.
type HookEvent struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// Logger is the logging interface exposed to plugins.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// BasePlugin provides default implementations of Plugin methods.
// Embed this in plugin structs to reduce boilerplate.
type BasePlugin struct{}

// Init is a no-op default implementation.
func (BasePlugin) Init(Host) error { return nil }

// Shutdown is a no-op default implementation.
func (BasePlugin) Shutdown() error { return nil }

// Validate checks that a plugin's metadata is complete.
func Validate(p Plugin) error {
	meta := p.Metadata()
	if meta.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if meta.Version == "" {
		return fmt.Errorf("plugin version is required")
	}
	if meta.Description == "" {
		return fmt.Errorf("plugin description is required")
	}
	return nil
}
