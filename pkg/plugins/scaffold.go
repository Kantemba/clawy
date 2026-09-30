package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kantemba/clawy/pkg/skills"
)

// ScaffoldResult describes a freshly created plugin skeleton.
type ScaffoldResult struct {
	// Dir is the created plugin directory.
	Dir string
	// Files lists the created files, relative to Dir.
	Files []string
}

// Scaffold creates a Clawy-native plugin skeleton under destRoot/name:
//
//	<name>/clawy-plugin.json    manifest
//	<name>/skills/<name>/SKILL.md
//	<name>/commands/<name>.md
//	<name>/README.md
//
// The skeleton is immediately discoverable — dropping it into
// <workspace>/plugins is enough for the next startup (or `/reload`) to load
// it. An existing directory is never overwritten.
func Scaffold(destRoot, name, description string) (*ScaffoldResult, error) {
	name = strings.TrimSpace(name)
	if err := skills.ValidateSkillName(name); err != nil {
		return nil, fmt.Errorf("invalid plugin name %q: %w", name, err)
	}
	if destRoot == "" {
		return nil, fmt.Errorf("destination directory is required")
	}
	if strings.TrimSpace(description) == "" {
		description = "Clawy plugin " + name
	}

	dir := filepath.Join(destRoot, name)
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("%s already exists", dir)
		}
		entries, readErr := os.ReadDir(dir)
		if readErr == nil && len(entries) > 0 {
			return nil, fmt.Errorf("directory %s already exists and is not empty", dir)
		}
	}

	result := &ScaffoldResult{Dir: dir}
	files := []struct {
		relative string
		content  string
	}{
		{"clawy-plugin.json", manifestTemplate(name, description)},
		{filepath.Join("skills", name, "SKILL.md"), skillTemplate(name, description)},
		{filepath.Join("commands", name+".md"), commandTemplate(name, description)},
		{"README.md", readmeTemplate(name, description)},
	}

	for _, file := range files {
		path := filepath.Join(dir, file.relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(file.content), 0o644); err != nil {
			return nil, err
		}
		result.Files = append(result.Files, file.relative)
	}

	return result, nil
}

func manifestTemplate(name, description string) string {
	manifest := map[string]any{
		"name":        name,
		"version":     "0.1.0",
		"description": description,
		"skills":      "./skills/",
		"commands":    "./commands/",
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Sprintf("{\n  %q: %q\n}\n", "name", name)
	}
	return string(data) + "\n"
}

func skillTemplate(name, description string) string {
	return fmt.Sprintf(`---
name: %s
description: %s
---

# %s

Describe what this skill does and when to use it.

## Steps

1. Replace this list with the actual procedure.
2. Keep each step short and deterministic.
3. Reference files by path so the agent can read them.
`, name, description, name)
}

func commandTemplate(name, description string) string {
	return fmt.Sprintf(`---
description: %s
argument-hint: "[topic]"
---

# %s

Run the %s workflow for the topic below.

Topic: {{args}}
`, description, name, name)
}

func readmeTemplate(name, description string) string {
	return fmt.Sprintf(`# %s

%s

## Surfaces

- `+"`skills/%s/SKILL.md`"+` — skill injected into the agent catalog
- `+"`commands/%s.md`"+` — slash command /%s (prompt command)
- optional: `+"`.mcp.json`"+` (MCP servers) and `+"`hooks.json`"+` (hooks)

## Install

Copy this directory into `+"`<workspace>/plugins`"+` or `+"`~/.clawy/plugins`"+`
and run `+"`/reload`"+` (or restart the gateway).

Check it with `+"`clawy plugin list`"+`.
`, name, description, name, name, name)
}
