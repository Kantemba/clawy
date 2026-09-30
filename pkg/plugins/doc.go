// Package plugins implements Clawy's directory-based plugin system.
//
// A plugin is a directory that carries a manifest and any combination of four
// surfaces:
//
//	skills/     skills/<name>/SKILL.md      — injected into the skill catalog
//	commands/   commands/<name>.md          — prompt slash commands
//	hooks.json  processes/builtins          — mounted by the hook runtime
//	.mcp.json   {"mcpServers": {...}}       — connected at startup
//
// Two manifest dialects are understood:
//
//	Codex-compatible  <plugin>/.codex-plugin/plugin.json
//	Clawy-native      <plugin>/clawy-plugin.json  (or .clawy-plugin/plugin.json)
//
// The Codex dialect is what https://github.com/openai/plugins ships, so a
// clone of that repository dropped into any search root works unchanged:
//
//	~/.clawy/plugins/openai/plugins/plugins/notion/.codex-plugin/plugin.json
//
// Plugins are discovered by Bootstrap (or Discover for read-only inspection)
// from $CLAWY_PLUGINS, config plugins.dirs, <workspace>/plugins and
// ~/.clawy/plugins. Bootstrap merges plugin MCP servers and hooks into the
// config, publishes the plugin skill roots to pkg/skills, and activates the
// bundle so the agent loop can register plugin slash commands.
//
// Nothing in this package fails startup: a malformed plugin is recorded as a
// warning in Bundle.Warnings and skipped.
package plugins
