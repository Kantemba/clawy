package plugins

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Kantemba/clawy/pkg/commands"
	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/logger"
	"github.com/Kantemba/clawy/pkg/skills"
)

// Bundle is the set of plugins loaded for this process.
type Bundle struct {
	// Plugins are the successfully loaded plugins, sorted by name.
	Plugins []*Plugin

	// SkillRoots are all skill roots contributed by the plugins, in plugin
	// order. They are published to pkg/skills by Bootstrap.
	SkillRoots []string

	// Warnings collects non-fatal problems (bad manifests, broken surfaces,
	// name collisions). They are logged at startup and shown by
	// `clawy plugin list`.
	Warnings []string
}

// finish normalizes the bundle after discovery.
func (b *Bundle) finish() {
	b.SkillRoots = b.SkillRoots[:0]
	seen := map[string]struct{}{}
	for _, plugin := range b.Plugins {
		for _, root := range plugin.SkillRoots {
			key := canonicalDir(root)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			b.SkillRoots = append(b.SkillRoots, root)
		}
	}
}

// CommandDefinitions returns every slash command contributed by the plugins.
func (b *Bundle) CommandDefinitions() []commands.Definition {
	if b == nil {
		return nil
	}
	var defs []commands.Definition
	for _, plugin := range b.Plugins {
		defs = append(defs, plugin.Commands...)
	}
	return defs
}

// Find returns a plugin by name (case-insensitive).
func (b *Bundle) Find(name string) (*Plugin, bool) {
	if b == nil {
		return nil, false
	}
	wanted := strings.ToLower(strings.TrimSpace(name))
	for _, plugin := range b.Plugins {
		if strings.ToLower(plugin.Name()) == wanted {
			return plugin, true
		}
	}
	return nil, false
}

// Counts summarizes the loaded surfaces for startup logging.
func (b *Bundle) Counts() (commands, servers, hooks int) {
	if b == nil {
		return 0, 0, 0
	}
	for _, plugin := range b.Plugins {
		commands += len(plugin.Commands)
		servers += len(plugin.MCPServers)
		hooks += len(plugin.Hooks.Processes) + len(plugin.Hooks.Builtins)
	}
	return commands, servers, hooks
}

// IsEmpty reports whether the bundle carries nothing.
func (b *Bundle) IsEmpty() bool {
	return b == nil || len(b.Plugins) == 0
}

var (
	activeMu     sync.RWMutex
	activeBundle = &Bundle{}
)

// Active returns the bundle installed by the most recent Bootstrap. It never
// returns nil: before the first Bootstrap it is an empty bundle, so callers
// can use it unconditionally.
func Active() *Bundle {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeBundle
}

func activate(bundle *Bundle) {
	if bundle == nil {
		bundle = &Bundle{}
	}
	activeMu.Lock()
	activeBundle = bundle
	activeMu.Unlock()
}

// Bootstrap discovers plugins, merges their MCP servers and hooks into cfg,
// publishes their skill roots and activates the bundle so the agent loop can
// register plugin slash commands.
//
// It is safe to call again on config reload with a freshly loaded cfg.
// Discovery never fails the startup: problems are logged and returned as
// Bundle.Warnings.
func Bootstrap(cfg *config.Config) *Bundle {
	if cfg == nil || !cfg.Plugins.Enabled {
		deactivate()
		return &Bundle{}
	}

	bundle := Discover(cfg)
	bundle.applyToConfig(cfg)
	skills.SetPluginSkillRoots(bundle.SkillRoots)
	activate(bundle)

	commandCount, serverCount, hookCount := bundle.Counts()
	if len(bundle.Plugins) > 0 {
		logger.InfoCF("plugins", "Plugins loaded",
			map[string]any{
				"plugins":  len(bundle.Plugins),
				"skills":   len(bundle.SkillRoots),
				"commands": commandCount,
				"mcp":      serverCount,
				"hooks":    hookCount,
			})
	}
	for _, warning := range bundle.Warnings {
		logger.WarnCF("plugins", warning, nil)
	}

	return bundle
}

// deactivate clears every plugin contribution (used when plugins are
// disabled in config).
func deactivate() {
	activate(nil)
	skills.SetPluginSkillRoots(nil)
}

// applyToConfig merges plugin MCP servers and hooks into cfg. The merge is
// non-destructive: a user-configured entry always wins over the plugin one.
func (b *Bundle) applyToConfig(cfg *config.Config) {
	if b == nil || cfg == nil {
		return
	}
	b.applyMCP(cfg)
	b.applyHooks(cfg)
}

// applyMCP merges plugin MCP servers into cfg.Tools.MCP.Servers. Names that
// clash with a user-configured server are namespaced with the plugin name;
// the plugin is dropped only when even the namespaced name is taken.
func (b *Bundle) applyMCP(cfg *config.Config) {
	for _, plugin := range b.Plugins {
		for name, server := range plugin.MCPServers {
			if cfg.Tools.MCP.Servers == nil {
				cfg.Tools.MCP.Servers = map[string]config.MCPServerConfig{}
			}

			resolved := name
			if _, exists := cfg.Tools.MCP.Servers[resolved]; exists {
				resolved = plugin.Name() + "-" + name
				if _, exists := cfg.Tools.MCP.Servers[resolved]; exists {
					logger.WarnCF("plugins", "Plugin MCP server name already in use",
						map[string]any{"plugin": plugin.Name(), "server": name})
					continue
				}
			}

			cfg.Tools.MCP.Servers[resolved] = server
		}
	}
}

// applyHooks merges plugin hooks into cfg.Hooks. Process hooks are namespaced
// only on collision (their names are arbitrary); builtin hook names must stay
// verbatim because they key the builtin factory registry.
func (b *Bundle) applyHooks(cfg *config.Config) {
	for _, plugin := range b.Plugins {
		if len(plugin.Hooks.Processes) == 0 && len(plugin.Hooks.Builtins) == 0 {
			continue
		}

		if cfg.Hooks.Processes == nil {
			cfg.Hooks.Processes = map[string]config.ProcessHookConfig{}
		}
		if cfg.Hooks.Builtins == nil {
			cfg.Hooks.Builtins = map[string]config.BuiltinHookConfig{}
		}

		for name, hook := range plugin.Hooks.Processes {
			resolved := name
			if _, exists := cfg.Hooks.Processes[resolved]; exists {
				resolved = plugin.Name() + "-" + name
				if _, exists := cfg.Hooks.Processes[resolved]; exists {
					logger.WarnCF("plugins", "Plugin hook name already in use",
						map[string]any{"plugin": plugin.Name(), "hook": name})
					continue
				}
			}
			cfg.Hooks.Processes[resolved] = hook
		}

		for name, hook := range plugin.Hooks.Builtins {
			if _, exists := cfg.Hooks.Builtins[name]; exists {
				logger.WarnCF("plugins", "Plugin builtin hook already configured",
					map[string]any{"plugin": plugin.Name(), "hook": name})
				continue
			}
			cfg.Hooks.Builtins[name] = hook
		}
	}
}

// Describe renders a one-line summary for CLI output.
func (b *Bundle) Describe() string {
	if b.IsEmpty() {
		return "no plugins"
	}
	commandCount, serverCount, _ := b.Counts()
	return fmt.Sprintf("%d plugin(s), %d skill root(s), %d command(s), %d MCP server(s)",
		len(b.Plugins), len(b.SkillRoots), commandCount, serverCount)
}

// sortedNames returns plugin names sorted alphabetically (CLI helpers).
func (b *Bundle) sortedNames() []string {
	if b == nil {
		return nil
	}
	names := make([]string, 0, len(b.Plugins))
	for _, plugin := range b.Plugins {
		names = append(names, plugin.Name())
	}
	sort.Strings(names)
	return names
}
