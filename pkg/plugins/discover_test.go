package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/skills"
)

// testConfig returns a config rooted in a temp home so discovery never sees
// the developer's real ~/.clawy/plugins.
func testConfig(t *testing.T) *config.Config {
	t.Helper()

	home := t.TempDir()
	t.Setenv(config.EnvHome, home)
	t.Setenv(config.EnvPlugins, "")

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(home, "workspace")
	return cfg
}

// writeCodexPlugin creates a Codex-dialect plugin with every surface.
func writeCodexPlugin(t *testing.T, pluginDir, name string) string {
	t.Helper()

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	manifest := `{
  "name": "` + name + `",
  "version": "1.0.0",
  "description": "test plugin",
  "author": {"name": "Tester"},
  "skills": "./skills/",
  "mcpServers": "./.mcp.json"
}`
	writeManifest(t, pluginDir, ".codex-plugin/plugin.json", manifest)

	skillDir := filepath.Join(pluginDir, "skills", name+"-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	skill := "---\nname: " + name + "-skill\ndescription: a test skill\n---\n\n# " + name + "-skill\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	cmdDir := filepath.Join(pluginDir, "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("mkdir commands: %v", err)
	}
	command := "---\ndescription: run the " + name + " workflow\nargument-hint: \"[target]\"\n---\n\nDo the " + name + " thing for {{args}}\n"
	if err := os.WriteFile(filepath.Join(cmdDir, name+".md"), []byte(command), 0o644); err != nil {
		t.Fatalf("write command: %v", err)
	}

	mcp := `{"mcpServers": {"` + name + `-echo": {"command": "node", "args": ["server.js"]}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, ".mcp.json"), []byte(mcp), 0o644); err != nil {
		t.Fatalf("write mcp: %v", err)
	}

	hooks := `{"processes": {"` + name + `-gate": {"command": ["node", "hook.js"], "observe": ["after_tool"]}}}`
	if err := os.WriteFile(filepath.Join(pluginDir, "hooks.json"), []byte(hooks), 0o644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}

	return pluginDir
}

func TestDiscoverWorkspacePlugins(t *testing.T) {
	cfg := testConfig(t)
	pluginDir := writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "notion"), "notion")

	bundle := Discover(cfg)
	if len(bundle.Plugins) != 1 {
		t.Fatalf("Discover() loaded %d plugins, want 1 (%+v)", len(bundle.Plugins), bundle.Warnings)
	}

	plugin := bundle.Plugins[0]
	if plugin.Name() != "notion" {
		t.Errorf("Name() = %q, want notion", plugin.Name())
	}
	if plugin.Format() != FormatCodex {
		t.Errorf("Format() = %q, want %q", plugin.Format(), FormatCodex)
	}
	if plugin.Dir != pluginDir {
		t.Errorf("Dir = %q, want %q", plugin.Dir, pluginDir)
	}
	if len(plugin.SkillRoots) != 1 {
		t.Fatalf("SkillRoots = %v, want one root", plugin.SkillRoots)
	}
	if len(bundle.SkillRoots) != 1 {
		t.Errorf("bundle.SkillRoots = %v, want one root", bundle.SkillRoots)
	}
	if len(plugin.Commands) != 1 || plugin.Commands[0].Name != "notion" {
		t.Errorf("Commands = %+v, want one command named notion", plugin.Commands)
	}
	if plugin.Commands[0].Prompt == "" {
		t.Error("command prompt body is empty")
	}
	if _, ok := plugin.MCPServers["notion-echo"]; !ok {
		t.Errorf("MCPServers = %v, want notion-echo", plugin.MCPServers)
	}
	if len(plugin.Hooks.Processes) != 1 {
		t.Errorf("Hooks.Processes = %v, want one process hook", plugin.Hooks.Processes)
	}
	if len(bundle.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", bundle.Warnings)
	}
}

func TestDiscoverClawyDialectFromEnvRoot(t *testing.T) {
	cfg := testConfig(t)

	extraRoot := filepath.Join(t.TempDir(), "extra")
	pluginDir := filepath.Join(extraRoot, "hello")
	writeManifest(t, pluginDir, "clawy-plugin.json",
		`{"name": "hello", "version": "0.2.0", "description": "hello plugin"}`)
	if err := os.MkdirAll(filepath.Join(pluginDir, "commands"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "commands", "hello.md"),
		[]byte("---\ndescription: say hello\n---\n\nSay hello to {{args}}\n"), 0o644); err != nil {
		t.Fatalf("write command: %v", err)
	}

	t.Setenv(config.EnvPlugins, extraRoot)

	bundle := Discover(cfg)
	if len(bundle.Plugins) != 1 {
		t.Fatalf("Discover() loaded %d plugins, want 1 (%+v)", len(bundle.Plugins), bundle.Warnings)
	}
	if bundle.Plugins[0].Format() != FormatClawy {
		t.Errorf("Format() = %q, want %q", bundle.Plugins[0].Format(), FormatClawy)
	}
	if len(bundle.Plugins[0].Commands) != 1 {
		t.Errorf("Commands = %+v, want one", bundle.Plugins[0].Commands)
	}
}

func TestDiscoverRepositoryLayout(t *testing.T) {
	cfg := testConfig(t)

	// github.com/openai/plugins style checkout: <root>/plugins/<name>/...
	clone := filepath.Join(cfg.WorkspacePath(), "plugins", "openai-plugins")
	writeCodexPlugin(t, filepath.Join(clone, "plugins", "figma"), "figma")
	writeCodexPlugin(t, filepath.Join(clone, "plugins", "netlify"), "netlify")

	bundle := Discover(cfg)
	if len(bundle.Plugins) != 2 {
		t.Fatalf("Discover() loaded %d plugins, want 2 (%+v)", len(bundle.Plugins), bundle.Warnings)
	}
	if _, ok := bundle.Find("figma"); !ok {
		t.Error("figma not discovered")
	}
	if _, ok := bundle.Find("netlify"); !ok {
		t.Error("netlify not discovered")
	}
}

func TestDiscoverRespectsDisabledConfig(t *testing.T) {
	cfg := testConfig(t)
	writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "notion"), "notion")
	cfg.Plugins.Disable = []string{"NoTiOn"}

	bundle := Discover(cfg)
	if len(bundle.Plugins) != 0 {
		t.Fatalf("Discover() loaded %d plugins, want 0", len(bundle.Plugins))
	}
}

func TestDiscoverRespectsManifestEnabledFlag(t *testing.T) {
	cfg := testConfig(t)
	pluginDir := writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "notion"), "notion")
	writeManifest(t, pluginDir, ".codex-plugin/plugin.json",
		`{"name": "notion", "version": "1.0.0", "description": "off", "enabled": false}`)

	bundle := Discover(cfg)
	if len(bundle.Plugins) != 0 {
		t.Fatalf("Discover() loaded %d plugins, want 0", len(bundle.Plugins))
	}
}

func TestDiscoverDisabledGlobally(t *testing.T) {
	cfg := testConfig(t)
	writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "notion"), "notion")
	cfg.Plugins.Enabled = false

	bundle := Discover(cfg)
	if len(bundle.Plugins) != 0 {
		t.Fatalf("Discover() loaded %d plugins, want 0", len(bundle.Plugins))
	}
}

func TestDiscoverDuplicateNamesKeepFirst(t *testing.T) {
	cfg := testConfig(t)
	writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "first"), "duplicate")
	writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "second"), "duplicate")

	bundle := Discover(cfg)
	if len(bundle.Plugins) != 1 {
		t.Fatalf("Discover() loaded %d plugins, want 1", len(bundle.Plugins))
	}
	if len(bundle.Warnings) == 0 {
		t.Error("expected a duplicate-name warning")
	}
}

func TestDiscoverBrokenManifestIsWarningNotFailure(t *testing.T) {
	cfg := testConfig(t)
	pluginDir := filepath.Join(cfg.WorkspacePath(), "plugins", "broken")
	writeManifest(t, pluginDir, "clawy-plugin.json", `{oops`)
	writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "good"), "good")

	bundle := Discover(cfg)
	if len(bundle.Plugins) != 1 {
		t.Fatalf("Discover() loaded %d plugins, want the healthy one (%v)", len(bundle.Plugins), bundle.Warnings)
	}
	if len(bundle.Warnings) == 0 {
		t.Error("expected a warning for the broken manifest")
	}
}

func TestBootstrapActivatesSkillsAndCommands(t *testing.T) {
	cfg := testConfig(t)
	writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "notion"), "notion")
	t.Cleanup(func() { skills.SetPluginSkillRoots(nil) })

	bundle := Bootstrap(cfg)
	if bundle.IsEmpty() {
		t.Fatal("Bootstrap() returned an empty bundle")
	}
	if Active() != bundle {
		t.Error("Active() does not return the bootstrapped bundle")
	}

	roots := skills.PluginSkillRoots()
	if len(roots) != 1 || roots[0] != bundle.SkillRoots[0] {
		t.Errorf("skills.PluginSkillRoots() = %v, want %v", roots, bundle.SkillRoots)
	}

	defs := Active().CommandDefinitions()
	if len(defs) != 1 || defs[0].Name != "notion" {
		t.Errorf("CommandDefinitions() = %+v, want the notion command", defs)
	}

	// The plugin's MCP server and hook must be merged into the config.
	server, ok := cfg.Tools.MCP.Servers["notion-echo"]
	if !ok {
		t.Fatalf("MCP servers = %v, want notion-echo", cfg.Tools.MCP.Servers)
	}
	if !server.Enabled {
		t.Error("plugin MCP server should default to enabled")
	}
	if _, ok := cfg.Hooks.Processes["notion-gate"]; !ok {
		t.Errorf("Hooks.Processes = %v, want notion-gate", cfg.Hooks.Processes)
	}
}

func TestBootstrapDisabledClearsContributions(t *testing.T) {
	cfg := testConfig(t)
	writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "notion"), "notion")
	t.Cleanup(func() { skills.SetPluginSkillRoots(nil) })

	Bootstrap(cfg)
	if len(skills.PluginSkillRoots()) == 0 {
		t.Fatal("expected plugin skill roots after Bootstrap")
	}

	cfg.Plugins.Enabled = false
	Bootstrap(cfg)

	if len(skills.PluginSkillRoots()) != 0 {
		t.Errorf("skill roots = %v, want cleared", skills.PluginSkillRoots())
	}
	if !Active().IsEmpty() {
		t.Error("Active() bundle should be empty when plugins are disabled")
	}
}

func TestApplyMCPNamespacesCollisions(t *testing.T) {
	cfg := testConfig(t)
	pluginDir := writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "notion"), "notion")

	// A user-configured server already owns the bare name.
	cfg.Tools.MCP.Servers = map[string]config.MCPServerConfig{
		"notion-echo": {Enabled: true, Command: "user-owned"},
	}

	bundle := Discover(cfg)
	bundle.applyToConfig(cfg)

	server, ok := cfg.Tools.MCP.Servers["notion-notion-echo"]
	if !ok {
		t.Fatalf("MCP servers = %v, want namespaced notion-notion-echo (%s)", cfg.Tools.MCP.Servers, pluginDir)
	}
	if server.Command != "node" {
		t.Errorf("namespaced server should come from the plugin, got %+v", server)
	}
	if cfg.Tools.MCP.Servers["notion-echo"].Command != "user-owned" {
		t.Error("user-configured server must not be overwritten")
	}
}

func TestSearchRootsOrder(t *testing.T) {
	cfg := testConfig(t)
	extra := t.TempDir()
	t.Setenv(config.EnvPlugins, extra)

	roots := SearchRoots(cfg)
	if len(roots) < 3 {
		t.Fatalf("SearchRoots() = %v, want env + workspace + home roots", roots)
	}
	if roots[0] != extra {
		t.Errorf("first root = %q, want the $CLAWY_PLUGINS entry %q", roots[0], extra)
	}
	if roots[1] != filepath.Join(cfg.WorkspacePath(), "plugins") {
		t.Errorf("second root = %q, want workspace plugins", roots[1])
	}
	if roots[2] != filepath.Join(config.GetHome(), "plugins") {
		t.Errorf("third root = %q, want home plugins", roots[2])
	}
}
