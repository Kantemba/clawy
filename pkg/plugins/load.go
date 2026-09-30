package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Kantemba/clawy/pkg/commands"
	"github.com/Kantemba/clawy/pkg/config"
)

// Plugin is a discovered plugin directory together with everything it
// contributes to the running agent.
type Plugin struct {
	Manifest *Manifest
	Dir      string

	// SkillRoots are directories whose children are skill directories
	// (skills/<name>/SKILL.md).
	SkillRoots []string

	// Commands are slash commands contributed by commands/*.md. They carry a
	// Prompt body instead of a Go handler.
	Commands []commands.Definition

	// Hooks holds the process/builtin hooks declared by hooks.json.
	Hooks config.HooksConfig

	// MCPServers holds the MCP servers declared by .mcp.json (or the inline
	// manifest map), already normalized to Clawy's config shape with
	// Enabled defaulted to true.
	MCPServers map[string]config.MCPServerConfig
}

// Name returns the plugin's unique name.
func (p *Plugin) Name() string {
	if p.Manifest == nil {
		return ""
	}
	return p.Manifest.Name
}

// Version returns the plugin version.
func (p *Plugin) Version() string {
	if p.Manifest == nil {
		return ""
	}
	return p.Manifest.Version
}

// Description returns the plugin description.
func (p *Plugin) Description() string {
	if p.Manifest == nil {
		return ""
	}
	return p.Manifest.Description
}

// Format returns the manifest dialect ("codex" or "clawy").
func (p *Plugin) Format() string {
	if p.Manifest == nil {
		return ""
	}
	return p.Manifest.Format
}

// Surfaces summarizes what the plugin contributes, for CLI display.
func (p *Plugin) Surfaces() []string {
	var out []string
	if len(p.SkillRoots) > 0 {
		out = append(out, fmt.Sprintf("skills(%d)", countSkills(p.SkillRoots)))
	}
	if len(p.Commands) > 0 {
		out = append(out, fmt.Sprintf("commands(%d)", len(p.Commands)))
	}
	if len(p.MCPServers) > 0 {
		out = append(out, fmt.Sprintf("mcp(%d)", len(p.MCPServers)))
	}
	if len(p.Hooks.Processes) > 0 || len(p.Hooks.Builtins) > 0 {
		out = append(out, fmt.Sprintf("hooks(%d)", len(p.Hooks.Processes)+len(p.Hooks.Builtins)))
	}
	if len(out) == 0 {
		return []string{"manifest-only"}
	}
	return out
}

// loadPlugin parses the manifest and resolves every surface it declares.
// Non-fatal surface problems are appended to warnings instead of failing the
// plugin, so one broken file cannot take a plugin (or the gateway) down.
func loadPlugin(manifestPath string, disabled map[string]struct{}) (*Plugin, []string, error) {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, nil, err
	}
	if !manifest.IsEnabled() {
		return nil, nil, nil
	}
	if _, ok := disabled[strings.ToLower(manifest.Name)]; ok {
		return nil, nil, nil
	}

	plugin := &Plugin{
		Manifest:   manifest,
		Dir:        manifest.Dir,
		MCPServers: map[string]config.MCPServerConfig{},
	}

	var warnings []string
	warnf := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf("%s: %s", manifest.Name, fmt.Sprintf(format, args...)))
	}

	plugin.SkillRoots = resolveSkillRoots(manifest)
	plugin.Commands, warnings = resolveCommands(manifest)

	if err := plugin.resolveMCP(); err != nil {
		warnf("mcp servers skipped: %v", err)
	}
	if err := plugin.resolveHooks(); err != nil {
		warnf("hooks skipped: %v", err)
	}

	return plugin, warnings, nil
}

// resolveSkillRoots returns the plugin directories that contain at least one
// skill. An empty (or conventionally named but missing) skills directory is
// not an error: plenty of plugins only ship commands or MCP servers.
func resolveSkillRoots(manifest *Manifest) []string {
	entries := []string(manifest.Skills)
	if len(entries) == 0 {
		entries = []string{"skills"}
	}

	var roots []string
	for _, entry := range entries {
		dir := resolvePath(manifest.Dir, entry)
		if dir == "" {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		if !dirHasSkill(dir) {
			continue
		}
		roots = append(roots, dir)
	}
	return roots
}

// dirHasSkill reports whether any child of root is a skill directory
// (contains SKILL.md) or the root itself holds a SKILL.md.
func dirHasSkill(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
		return true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "SKILL.md")); err == nil {
			return true
		}
	}
	return false
}

func countSkills(roots []string) int {
	total := 0
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, entry.Name(), "SKILL.md")); err == nil {
				total++
			}
		}
	}
	return total
}

// commandFrontmatter is the YAML frontmatter of a plugin command file.
// Frontmatter written for other tools is tolerated: scalar-or-list values are
// accepted for every string field, and a block that fails to parse outright is
// ignored instead of dropping the command.
type commandFrontmatter struct {
	Name        yamlScalar     `yaml:"name"`
	Description yamlScalar     `yaml:"description"`
	Usage       yamlScalar     `yaml:"usage"`
	ArgsUsage   yamlScalar     `yaml:"argument-hint"`
	Args        yamlScalar     `yaml:"args"`
	Aliases     yamlStringList `yaml:"aliases"`
	Alias       yamlScalar     `yaml:"alias"`
}

// yamlScalar accepts a YAML scalar or a sequence of scalars (joined with a
// space). Other node kinds resolve to an empty value.
type yamlScalar string

func (s *yamlScalar) UnmarshalYAML(node *yaml.Node) error {
	if node != nil && node.Kind == yaml.SequenceNode {
		var parts []string
		for _, item := range node.Content {
			if text := scalarText(item); text != "" {
				parts = append(parts, text)
			}
		}
		*s = yamlScalar(strings.Join(parts, " "))
		return nil
	}
	*s = yamlScalar(scalarText(node))
	return nil
}

// yamlStringList accepts a single value or a list of values.
type yamlStringList []string

func (l *yamlStringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			if text := strings.TrimSpace(scalarText(item)); text != "" {
				out = append(out, text)
			}
		}
		*l = out
	default:
		if text := strings.TrimSpace(scalarText(node)); text != "" {
			*l = []string{text}
		} else {
			*l = nil
		}
	}
	return nil
}

func scalarText(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

// resolveCommands loads every command definition contributed by the plugin.
func resolveCommands(manifest *Manifest) ([]commands.Definition, []string) {
	entries := []string(manifest.Commands)
	if len(entries) == 0 {
		entries = []string{"commands"}
	}

	var defs []commands.Definition
	var warnings []string

	for _, entry := range entries {
		path := resolvePath(manifest.Dir, entry)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		files := []string{}
		if info.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: cannot read commands dir %s: %v", manifest.Name, path, err))
				continue
			}
			for _, fileEntry := range entries {
				if fileEntry.IsDir() || !isCommandFile(fileEntry.Name()) {
					continue
				}
				files = append(files, filepath.Join(path, fileEntry.Name()))
			}
		} else if isCommandFile(filepath.Base(path)) {
			files = append(files, path)
		}

		for _, file := range files {
			def, err := loadCommandFile(file)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", manifest.Name, err))
				continue
			}
			if def == nil {
				continue
			}
			defs = append(defs, *def)
		}
	}

	return defs, warnings
}

// isCommandFile keeps markdown command files and drops README/license noise.
func isCommandFile(name string) bool {
	if !strings.EqualFold(filepath.Ext(name), ".md") {
		return false
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	lower := strings.ToLower(base)
	if strings.HasPrefix(base, "_") || strings.HasPrefix(base, ".") {
		return false
	}
	switch lower {
	case "readme", "license", "changelog":
		return false
	}
	return true
}

// loadCommandFile turns a markdown file into a prompt command definition.
// The file body becomes Definition.Prompt, which the agent expands with the
// user's arguments and forwards to the LLM.
func loadCommandFile(path string) (*commands.Definition, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read command %s: %w", path, err)
	}

	frontmatter, body := splitFrontmatter(string(content))
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("command %s has an empty body", path)
	}

	var meta commandFrontmatter
	if frontmatter != "" {
		if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
			// A block foreign to our schema (tab indentation, weird tags…)
			// must not discard the command: fall back to the file name and
			// an empty description.
			meta = commandFrontmatter{}
		}
	}

	name := strings.TrimSpace(string(meta.Name))
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if strings.ContainsAny(name, "/ ") {
		return nil, fmt.Errorf("command %s has an invalid name %q", path, name)
	}

	aliases := []string(meta.Aliases)
	if alias := strings.TrimSpace(string(meta.Alias)); alias != "" {
		aliases = append(aliases, alias)
	}

	usage := strings.TrimSpace(string(meta.Usage))
	if usage == "" {
		usage = strings.TrimSpace(string(meta.ArgsUsage))
	}

	description := strings.TrimSpace(string(meta.Description))
	if description == "" {
		description = deriveDescription(body)
	}

	return &commands.Definition{
		Name:        name,
		Description: description,
		Usage:       usage,
		Aliases:     aliases,
		Prompt:      body,
	}, nil
}

// deriveDescription pulls a one-line summary out of a command file that has
// no frontmatter: the paragraph directly after the leading H1. This is the
// shape used by foreign repos (openai/plugins ships many such files).
func deriveDescription(body string) string {
	var (
		paragraph []string
		started   bool
	)

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case !started:
			if strings.HasPrefix(trimmed, "# ") {
				started = true
				continue
			}
			if trimmed == "" {
				continue
			}
			return "" // content before any heading: leave the summary empty
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			if len(paragraph) > 0 {
				return truncateWords(strings.Join(paragraph, " "), 200)
			}
			if strings.HasPrefix(trimmed, "#") {
				return "" // a heading straight after the H1: nothing to summarize
			}
		default:
			paragraph = append(paragraph, trimmed)
		}
	}

	return truncateWords(strings.Join(paragraph, " "), 200)
}

func truncateWords(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}

	head := runes[:limit]
	for i := len(head) - 1; i >= 0; i-- {
		if head[i] == ' ' {
			return string(head[:i]) + "…"
		}
	}
	return string(head) + "…"
}

// splitFrontmatter separates a leading YAML frontmatter block from the body.
func splitFrontmatter(content string) (frontmatter, body string) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", content
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" || strings.TrimSpace(lines[i]) == "..." {
			end = i
			break
		}
	}
	if end == -1 {
		return "", content
	}

	frontmatter = strings.Join(lines[1:end], "\n")
	body = strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	return frontmatter, body
}

// mcpServerEntry is the per-server shape shared by .mcp.json files and the
// inline manifest map.
type mcpServerEntry struct {
	Enabled  *bool                  `json:"enabled,omitempty"`
	Command  json.RawMessage        `json:"command,omitempty"`
	Args     []string               `json:"args,omitempty"`
	Env      map[string]string      `json:"env,omitempty"`
	EnvFile  string                 `json:"env_file,omitempty"`
	Type     string                 `json:"type,omitempty"`
	URL      string                 `json:"url,omitempty"`
	Headers  map[string]string      `json:"headers,omitempty"`
	OAuth    *config.MCPOAuthConfig `json:"oauth,omitempty"`
	Deferred *bool                  `json:"deferred,omitempty"`
}

// resolveMCP parses the plugin's MCP servers (.mcp.json by convention).
func (p *Plugin) resolveMCP() error {
	raw := p.Manifest.MCPServers
	if len(raw) == 0 {
		candidate := filepath.Join(p.Dir, ".mcp.json")
		data, err := os.ReadFile(candidate)
		if err != nil {
			// No MCP surface at all is perfectly normal.
			return nil
		}
		raw = data
	}

	return parseMCPServers(raw, p.Dir, p.MCPServers)
}

// parseMCPServers decodes an MCP server document into out. The document may
// be a path (string), a list of paths, a {"mcpServers": {...}} wrapper, or a
// bare {"<server>": {...}} map.
func parseMCPServers(raw json.RawMessage, pluginDir string, out map[string]config.MCPServerConfig) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}

	switch trimmed[0] {
	case '"', '[':
		var paths pathList
		if err := json.Unmarshal(raw, &paths); err != nil {
			return fmt.Errorf("invalid mcpServers path: %w", err)
		}
		for _, entry := range paths {
			path := resolvePath(pluginDir, entry)
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read mcp config %s: %w", path, err)
			}
			if err := parseMCPServers(data, filepath.Dir(path), out); err != nil {
				return err
			}
		}
		return nil
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return fmt.Errorf("invalid mcpServers object: %w", err)
	}

	// A fresh map: unmarshalling into the existing one would keep the wrapper
	// key alongside the servers it holds.
	var servers map[string]json.RawMessage
	if wrapped, ok := document["mcpServers"]; ok {
		if err := json.Unmarshal(wrapped, &servers); err != nil {
			return fmt.Errorf("invalid mcpServers.mcpServers: %w", err)
		}
	} else {
		servers = document
	}

	for name, rawEntry := range servers {
		var entry mcpServerEntry
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			return fmt.Errorf("server %q: %w", name, err)
		}
		server, err := entry.toConfig(pluginDir)
		if err != nil {
			return fmt.Errorf("server %q: %w", name, err)
		}
		out[name] = server
	}
	return nil
}

// toConfig normalizes a raw server entry into Clawy's config shape.
func (e mcpServerEntry) toConfig(pluginDir string) (config.MCPServerConfig, error) {
	command, args, err := splitCommand(e.Command, e.Args)
	if err != nil {
		return config.MCPServerConfig{}, err
	}
	if command == "" && strings.TrimSpace(e.URL) == "" {
		return config.MCPServerConfig{}, fmt.Errorf("needs a command or a url")
	}

	env := make(map[string]string, len(e.Env))
	for key, value := range e.Env {
		env[key] = expandEnvVars(value)
	}

	headers := make(map[string]string, len(e.Headers))
	for key, value := range e.Headers {
		headers[key] = expandEnvVars(value)
	}

	envFile := strings.TrimSpace(e.EnvFile)
	if envFile != "" && !filepath.IsAbs(envFile) {
		// Relative env files resolve against the plugin, not the workspace.
		if candidate := filepath.Join(pluginDir, envFile); fileExists(candidate) {
			envFile = candidate
		}
	}

	return config.MCPServerConfig{
		Enabled:  e.Enabled == nil || *e.Enabled,
		Deferred: e.Deferred,
		Command:  expandEnvVars(command),
		Args:     expandEnvList(args),
		Env:      env,
		EnvFile:  envFile,
		Type:     strings.TrimSpace(e.Type),
		URL:      expandEnvVars(strings.TrimSpace(e.URL)),
		Headers:  headers,
		OAuth:    e.OAuth,
	}, nil
}

// splitCommand accepts both `"command": "npx"` (with "args") and the array
// form `"command": ["npx", "-y", "server"]` used by several registries.
func splitCommand(raw json.RawMessage, args []string) (string, []string, error) {
	if len(raw) == 0 {
		return "", args, nil
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return strings.TrimSpace(single), args, nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return "", nil, fmt.Errorf("command must be a string or a list of strings")
	}
	if len(list) == 0 {
		return "", args, nil
	}
	if len(args) == 0 {
		args = list[1:]
	}
	return strings.TrimSpace(list[0]), args, nil
}

// hooksFile is the plugin hooks.json document.
type hooksFile struct {
	Processes map[string]rawProcessHook `json:"processes,omitempty"`
	Builtins  map[string]rawBuiltinHook `json:"builtins,omitempty"`
}

type rawProcessHook struct {
	Enabled   *bool             `json:"enabled,omitempty"`
	Priority  int               `json:"priority,omitempty"`
	Transport string            `json:"transport,omitempty"`
	Command   json.RawMessage   `json:"command,omitempty"`
	Dir       string            `json:"dir,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Observe   []string          `json:"observe,omitempty"`
	Intercept []string          `json:"intercept,omitempty"`
}

type rawBuiltinHook struct {
	Enabled  *bool           `json:"enabled,omitempty"`
	Priority int             `json:"priority,omitempty"`
	Config   json.RawMessage `json:"config,omitempty"`
}

// resolveHooks loads the plugin's hooks.json (or manifest-declared paths).
func (p *Plugin) resolveHooks() error {
	entries := []string(p.Manifest.Hooks)
	if len(entries) == 0 {
		entries = []string{"hooks.json"}
	}

	for _, entry := range entries {
		path := resolvePath(p.Dir, entry)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read hooks %s: %w", path, err)
		}
		if err := p.parseHooks(data, filepath.Dir(path)); err != nil {
			return fmt.Errorf("parse hooks %s: %w", path, err)
		}
	}
	return nil
}

func (p *Plugin) parseHooks(data []byte, hookDir string) error {
	var document hooksFile
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}

	if p.Hooks.Processes == nil {
		p.Hooks.Processes = map[string]config.ProcessHookConfig{}
	}
	if p.Hooks.Builtins == nil {
		p.Hooks.Builtins = map[string]config.BuiltinHookConfig{}
	}

	for name, raw := range document.Processes {
		command, args, err := splitCommand(raw.Command, nil)
		if err != nil {
			return fmt.Errorf("process hook %q: %w", name, err)
		}
		if command == "" {
			return fmt.Errorf("process hook %q: command is required", name)
		}
		dir := strings.TrimSpace(raw.Dir)
		if dir == "" {
			dir = hookDir
		}
		env := make(map[string]string, len(raw.Env))
		for key, value := range raw.Env {
			env[key] = expandEnvVars(value)
		}
		p.Hooks.Processes[name] = config.ProcessHookConfig{
			Enabled:   raw.Enabled == nil || *raw.Enabled,
			Priority:  raw.Priority,
			Transport: strings.TrimSpace(raw.Transport),
			Command:   append([]string{expandEnvVars(command)}, expandEnvList(args)...),
			Dir:       dir,
			Env:       env,
			Observe:   raw.Observe,
			Intercept: raw.Intercept,
		}
	}

	for name, raw := range document.Builtins {
		p.Hooks.Builtins[name] = config.BuiltinHookConfig{
			Enabled:  raw.Enabled == nil || *raw.Enabled,
			Priority: raw.Priority,
			Config:   raw.Config,
		}
	}
	return nil
}

// expandEnvVars expands $VAR / ${VAR} references from the process
// environment. Unknown variables are left literal so a plugin can reference
// shell-side syntax without silently losing data.
func expandEnvVars(value string) string {
	return os.Expand(value, func(key string) string {
		if resolved, ok := os.LookupEnv(key); ok {
			return resolved
		}
		return "${" + key + "}"
	})
}

func expandEnvList(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = expandEnvVars(value)
	}
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
