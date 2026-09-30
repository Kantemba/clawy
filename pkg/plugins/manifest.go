package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Supported manifest dialects.
const (
	// FormatCodex is the layout used by https://github.com/openai/plugins:
	// <plugin>/.codex-plugin/plugin.json.
	FormatCodex = "codex"

	// FormatClawy is Clawy's native layout: <plugin>/clawy-plugin.json or
	// <plugin>/.clawy-plugin/plugin.json.
	FormatClawy = "clawy"
)

// manifestRelPaths are the manifest locations probed inside a candidate
// directory, most specific first.
var manifestRelPaths = []string{
	filepath.Join(".codex-plugin", "plugin.json"),
	filepath.Join(".clawy-plugin", "plugin.json"),
	"clawy-plugin.json",
	"plugin.json",
}

// Author accepts both the string form ("author": "Ada Lovelace") and the
// object form used by the Codex plugin manifest.
type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// String renders the author for display.
func (a Author) String() string {
	switch {
	case a.Name != "" && a.Email != "":
		return a.Name + " <" + a.Email + ">"
	case a.Name != "":
		return a.Name
	case a.Email != "":
		return a.Email
	default:
		return ""
	}
}

func (a *Author) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		var name string
		if err := json.Unmarshal(trimmed, &name); err != nil {
			return err
		}
		a.Name = strings.TrimSpace(name)
		return nil
	}

	type authorAlias Author
	var alias authorAlias
	if err := json.Unmarshal(trimmed, &alias); err != nil {
		return err
	}
	*a = Author(alias)
	return nil
}

// pathList unmarshals a manifest field that may be a single path string, a
// list of paths, or null/absent. Unrecognized shapes are tolerated (and
// ignored) because manifests come from foreign repos that may model these
// fields differently.
type pathList []string

func (p *pathList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		*p = nil
		return nil
	}

	if trimmed[0] == '"' {
		var single string
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return err
		}
		if value := strings.TrimSpace(single); value != "" {
			*p = []string{value}
		} else {
			*p = nil
		}
		return nil
	}

	if trimmed[0] == '[' {
		var list []json.RawMessage
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return err
		}
		out := make(pathList, 0, len(list))
		for _, item := range list {
			var value string
			if err := json.Unmarshal(item, &value); err != nil {
				continue // tolerate list entries that are not strings
			}
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, value)
			}
		}
		*p = out
		return nil
	}

	// Foreign manifests use other shapes here — for example
	// "hooks": {} in openai/plugins. Treat anything unrecognized as
	// "no explicit paths" so the plugin still loads with its conventions.
	*p = nil
	return nil
}

// Manifest is a parsed plugin manifest. It covers the fields of the Codex
// plugin.json schema that map onto Clawy surfaces; unknown fields are
// tolerated and ignored so foreign manifests still load.
type Manifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	License     string   `json:"license,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Author      Author   `json:"author,omitempty"`

	// Skills, Commands and Hooks point at plugin surfaces. Each accepts a
	// path, a list of paths, or may be omitted to fall back to the
	// conventional location (skills/, commands/, hooks.json) when it exists.
	Skills   pathList `json:"skills,omitempty"`
	Commands pathList `json:"commands,omitempty"`
	Hooks    pathList `json:"hooks,omitempty"`

	// MCPServers is either a path to an MCP config file (the ".mcp.json"
	// convention: {"mcpServers": {...}}) or an inline server map. When
	// omitted, ./.mcp.json is used if it exists.
	MCPServers json.RawMessage `json:"mcpServers,omitempty"`

	// Enabled, when explicitly false, skips the plugin during discovery.
	Enabled *bool `json:"enabled,omitempty"`

	// Format is the manifest dialect ("codex" or "clawy").
	Format string `json:"-"`

	// Dir is the plugin root directory: the parent of the .codex-plugin or
	// .clawy-plugin folder when the manifest lives there.
	Dir string `json:"-"`

	// ManifestPath is the absolute path of the manifest file.
	ManifestPath string `json:"-"`
}

// IsEnabled reports whether the manifest does not explicitly disable itself.
func (m *Manifest) IsEnabled() bool {
	return m.Enabled == nil || *m.Enabled
}

// FindManifest returns the manifest path inside dir, or "" when the directory
// does not look like a plugin.
func FindManifest(dir string) string {
	for _, rel := range manifestRelPaths {
		candidate := filepath.Join(dir, rel)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return candidate
	}
	return ""
}

// LoadManifest reads and parses a plugin manifest.
func LoadManifest(path string) (*Manifest, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plugin manifest %s: %w", path, err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse plugin manifest %s: %w", path, err)
	}

	manifest.ManifestPath = absolute
	manifest.Dir = pluginDirFor(absolute)
	manifest.Format = formatFor(absolute)
	manifest.Name = strings.TrimSpace(manifest.Name)
	if manifest.Name == "" {
		manifest.Name = filepath.Base(manifest.Dir)
	}
	if strings.ContainsAny(manifest.Name, `/\`) {
		return nil, fmt.Errorf("plugin manifest %s: invalid name %q", path, manifest.Name)
	}
	if manifest.Version == "" {
		manifest.Version = "0.0.0"
	}

	return &manifest, nil
}

// pluginDirFor maps a manifest path to the plugin root directory.
func pluginDirFor(manifestPath string) string {
	dir := filepath.Dir(manifestPath)
	base := filepath.Base(dir)
	if base == ".codex-plugin" || base == ".clawy-plugin" {
		return filepath.Dir(dir)
	}
	return dir
}

func formatFor(manifestPath string) string {
	switch filepath.Base(filepath.Dir(manifestPath)) {
	case ".codex-plugin":
		return FormatCodex
	case ".clawy-plugin", "clawy-plugin.json", "plugin.json":
		return FormatClawy
	default:
		return FormatClawy
	}
}

// resolvePath resolves a manifest-relative surface entry against the plugin
// root. Absolute entries are kept as-is; "~" expands to the user home.
func resolvePath(pluginDir, entry string) string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return ""
	}
	if entry == "~" || strings.HasPrefix(entry, "~/") || strings.HasPrefix(entry, `~\`) {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			entry = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(entry, "~"), `/\`))
		}
	}
	if filepath.IsAbs(entry) {
		return filepath.Clean(entry)
	}
	return filepath.Join(pluginDir, entry)
}
