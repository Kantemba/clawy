package pluginsdk

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

// Manager loads, manages, and coordinates plugins.
type Manager struct {
	plugins   map[string]Plugin
	tools     map[string]ToolDefinition
	hooks     map[string][]HookHandler
	host      Host
	mu        sync.RWMutex
	ready     atomic.Bool
	pluginDir string
	logger    Logger
}

// NewManager creates a new plugin manager.
func NewManager(host Host, pluginDir string) *Manager {
	return &Manager{
		plugins:   make(map[string]Plugin),
		tools:     make(map[string]ToolDefinition),
		hooks:     make(map[string][]HookHandler),
		host:      host,
		pluginDir: pluginDir,
		logger:    host.Logger(),
	}
}

// Register adds a plugin directly (for in-process plugins).
func (m *Manager) Register(plugin Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := Validate(plugin); err != nil {
		return fmt.Errorf("validate plugin: %w", err)
	}

	meta := plugin.Metadata()
	name := meta.Name

	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin %q already registered", name)
	}

	if err := plugin.Init(m.host); err != nil {
		return fmt.Errorf("init plugin %q: %w", name, err)
	}

	m.plugins[name] = plugin

	// If it's a tool plugin, register its tools.
	if toolPlugin, ok := plugin.(ToolPlugin); ok {
		for _, def := range toolPlugin.Tools() {
			if _, exists := m.tools[def.Name]; exists {
				slog.Warn("plugin tool name collision", "tool", def.Name, "plugin", name)
				continue
			}
			m.tools[def.Name] = def
		}
	}

	// Register hooks.
	for _, hook := range meta.Hooks {
		m.hooks[hook] = append(m.hooks[hook], func(event HookEvent) error {
			return m.dispatchHook(plugin, event)
		})
	}

	m.logger.Info("plugin registered", "name", name, "version", meta.Version)
	return nil
}

// Unregister removes a plugin and its tools.
func (m *Manager) Unregister(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %q not found", name)
	}

	if err := plugin.Shutdown(); err != nil {
		m.logger.Warn("plugin shutdown error", "name", name, "error", err)
	}

	// Remove tools from tool plugins.
	if toolPlugin, ok := plugin.(ToolPlugin); ok {
		for _, def := range toolPlugin.Tools() {
			delete(m.tools, def.Name)
		}
	}

	delete(m.plugins, name)
	m.logger.Info("plugin unregistered", "name", name)
	return nil
}

// GetTool returns a tool definition by name.
func (m *Manager) GetTool(name string) (ToolDefinition, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tool, ok := m.tools[name]
	return tool, ok
}

// GetTools returns all registered plugin tools.
func (m *Manager) GetTools() []ToolDefinition {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tools := make([]ToolDefinition, 0, len(m.tools))
	for _, def := range m.tools {
		tools = append(tools, def)
	}
	return tools
}

// DispatchHook invokes all handlers for a given hook type.
func (m *Manager) DispatchHook(hookType string, event HookEvent) []error {
	m.mu.RLock()
	handlers := m.hooks[hookType]
	m.mu.RUnlock()

	var errs []error
	for _, handler := range handlers {
		if err := handler(event); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// Plugins returns metadata for all registered plugins.
func (m *Manager) Plugins() []PluginMeta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	metas := make([]PluginMeta, 0, len(m.plugins))
	for _, p := range m.plugins {
		metas = append(metas, p.Metadata())
	}
	return metas
}

// Shutdown unloads all plugins.
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for name, plugin := range m.plugins {
		if err := plugin.Shutdown(); err != nil {
			m.logger.Error("plugin shutdown failed", "name", name, "error", err)
			lastErr = err
		}
	}

	m.plugins = make(map[string]Plugin)
	m.tools = make(map[string]ToolDefinition)
	m.hooks = make(map[string][]HookHandler)
	return lastErr
}

func (m *Manager) dispatchHook(plugin Plugin, event HookEvent) error {
	// Default dispatch — plugins that want custom hook handling
	// should implement HookHandlerPlugin.
	if handler, ok := plugin.(HookHandlerPlugin); ok {
		return handler.HandleHook(event)
	}
	return nil
}

// HookHandlerPlugin allows plugins to directly handle hook events.
type HookHandlerPlugin interface {
	Plugin
	HandleHook(event HookEvent) error
}

// EnsureManagerReady marks the manager as ready after all plugins are loaded.
func (m *Manager) EnsureManagerReady() {
	m.ready.Store(true)
}

// IsReady reports whether the manager has been initialized.
func (m *Manager) IsReady() bool {
	return m.ready.Load()
}

// PluginConfig is the configuration format for loading plugins from disk.
type PluginConfig struct {
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config,omitempty"`
}
