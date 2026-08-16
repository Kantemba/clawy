package mcpserver

import (
	"fmt"

	"github.com/Kantemba/clawy/pkg/agent"
	"github.com/Kantemba/clawy/pkg/config"
	runtimeevents "github.com/Kantemba/clawy/pkg/events"
	toolshared "github.com/Kantemba/clawy/pkg/tools/shared"
)

// Factory creates native MCP Server instances from a running AgentLoop.
// It is the bridge between the agent's dynamic tool registry and the
// MCP protocol layer.
type Factory struct {
	agentLoop *agent.AgentLoop
	eventBus  runtimeevents.Bus
}

// NewFactory creates a Factory bound to the given AgentLoop.
func NewFactory(al *agent.AgentLoop) *Factory {
	f := &Factory{
		agentLoop: al,
	}
	if al != nil && al.RuntimeEventBus() != nil {
		f.eventBus = al.RuntimeEventBus()
	}
	return f
}

// WithEventBus overrides the event bus used for lifecycle events.
func (f *Factory) WithEventBus(bus runtimeevents.Bus) *Factory {
	f.eventBus = bus
	return f
}

// NewServer creates a native MCP Server using the agent loop's default
// agent tool registry as the tool provider.
func (f *Factory) NewServer() *Server {
	cfg := f.nativeConfig()
	return NewServer(
		cfg,
		f.ToolProvider(),
		WithEventBus(f.eventBus),
	)
}

// ToolProvider returns a ToolProvider backed by the default agent's
// tool registry. If the agent loop is nil or has no default agent, the
// returned provider returns an empty slice (graceful degradation).
func (f *Factory) ToolProvider() ToolProvider {
	return func() []toolshared.Tool {
		if f.agentLoop == nil {
			return nil
		}
		registry := f.agentLoop.GetRegistry()
		if registry == nil {
			return nil
		}
		instance := registry.GetDefaultAgent()
		if instance == nil || instance.Tools == nil {
			return nil
		}
		return instance.Tools.GetAll()
	}
}

// nativeConfig returns the effective NativeServerConfig from the agent
// loop's configuration, applying defaults if the agent loop is nil.
func (f *Factory) nativeConfig() config.NativeServerConfig {
	if f.agentLoop != nil {
		if cfg := f.agentLoop.GetConfig(); cfg != nil {
			return cfg.Tools.MCP.Native
		}
	}
	return config.NativeServerConfig{}
}

// NewServerFromConfig creates a native MCP Server from configuration alone,
// without a running AgentLoop. It registers the built-in file and exec
// tools that do not require an LLM provider.
//
// This is primarily used by the `clawy mcp serve` CLI command. If an
// AgentLoop is available (e.g. via gateway integration), prefer using
// Factory.NewServer() instead to expose the full tool set including
// MCP-connected tools and skills.
func NewServerFromConfig(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	toolProvider := buildStandaloneToolProvider(cfg)
	if toolProvider == nil {
		return nil, fmt.Errorf("failed to build tool provider from config")
	}

	srv := NewServer(
		cfg.Tools.MCP.Native,
		toolProvider,
		WithEventBus(runtimeevents.NewBus()),
	)

	return srv, nil
}

// buildStandaloneToolProvider registers the built-in tools that do not
// require an LLM provider, based on what is enabled in the config.
func buildStandaloneToolProvider(cfg *config.Config) ToolProvider {
	return func() []toolshared.Tool {
		// Defer to the agent package's own tool creation logic so that tool
		// constructors and config-driven options stay centralized.
		return agent.NewStandaloneToolRegistry(cfg).GetAll()
	}
}
