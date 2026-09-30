package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, manifestRel, content string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(manifestRel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func TestLoadManifestCodexDialect(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "notion")
	path := writeManifest(t, pluginDir, ".codex-plugin/plugin.json", `{
  "name": "notion",
  "version": "0.1.7",
  "description": "Notion workflows",
  "author": {"name": "Notion", "email": "support@openai.com"},
  "homepage": "https://www.notion.so/",
  "keywords": ["notion", "docs"],
  "skills": "./skills/",
  "mcpServers": "./.mcp.json",
  "apps": "./.app.json",
  "interface": {"category": "Productivity"}
}`)

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	if manifest.Format != FormatCodex {
		t.Errorf("Format = %q, want %q", manifest.Format, FormatCodex)
	}
	if manifest.Dir != pluginDir {
		t.Errorf("Dir = %q, want %q", manifest.Dir, pluginDir)
	}
	if manifest.Name != "notion" {
		t.Errorf("Name = %q, want %q", manifest.Name, "notion")
	}
	if manifest.Version != "0.1.7" {
		t.Errorf("Version = %q, want %q", manifest.Version, "0.1.7")
	}
	if manifest.Author.Name != "Notion" || manifest.Author.Email != "support@openai.com" {
		t.Errorf("Author = %+v, want name/email parsed", manifest.Author)
	}
	if len(manifest.Skills) != 1 || manifest.Skills[0] != "./skills/" {
		t.Errorf("Skills = %v, want [./skills/]", manifest.Skills)
	}
	if len(manifest.MCPServers) == 0 {
		t.Error("MCPServers raw is empty")
	}
	if !manifest.IsEnabled() {
		t.Error("IsEnabled() = false, want true")
	}
}

func TestLoadManifestClawyDialectStringAuthor(t *testing.T) {
	root := t.TempDir()
	path := writeManifest(t, root, "clawy-plugin.json", `{
  "name": "hello-clawy",
  "version": "1.2.3",
  "description": "demo",
  "author": "clawy",
  "commands": ["./commands/", "./extra.md"],
  "enabled": false
}`)

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	if manifest.Format != FormatClawy {
		t.Errorf("Format = %q, want %q", manifest.Format, FormatClawy)
	}
	if manifest.Dir != root {
		t.Errorf("Dir = %q, want %q", manifest.Dir, root)
	}
	if manifest.Author.Name != "clawy" {
		t.Errorf("Author.Name = %q, want %q", manifest.Author.Name, "clawy")
	}
	if len(manifest.Commands) != 2 {
		t.Errorf("Commands = %v, want 2 entries", manifest.Commands)
	}
	if manifest.IsEnabled() {
		t.Error("IsEnabled() = true, want false")
	}
}

func TestLoadManifestDefaultsNameAndVersion(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "my-plugin")
	path := writeManifest(t, pluginDir, "plugin.json", `{"description": "no name"}`)

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if manifest.Name != "my-plugin" {
		t.Errorf("Name = %q, want fallback to directory name", manifest.Name)
	}
	if manifest.Version != "0.0.0" {
		t.Errorf("Version = %q, want 0.0.0", manifest.Version)
	}
	if manifest.Dir != pluginDir {
		t.Errorf("Dir = %q, want %q", manifest.Dir, pluginDir)
	}
}

func TestLoadManifestRejectsInvalidJSON(t *testing.T) {
	root := t.TempDir()
	path := writeManifest(t, root, "clawy-plugin.json", `{ not json`)

	if _, err := LoadManifest(path); err == nil {
		t.Fatal("LoadManifest() error = nil, want parse error")
	}
}

func TestFindManifestPriority(t *testing.T) {
	root := t.TempDir()
	clawyPath := writeManifest(t, root, "clawy-plugin.json", `{"name": "clawy-form"}`)
	codexPath := writeManifest(t, root, ".codex-plugin/plugin.json", `{"name": "codex-form"}`)

	got := FindManifest(root)
	if got != codexPath {
		t.Errorf("FindManifest() = %q, want the codex manifest %q (clawy fallback: %q)", got, codexPath, clawyPath)
	}

	empty := t.TempDir()
	if got := FindManifest(empty); got != "" {
		t.Errorf("FindManifest(empty) = %q, want empty", got)
	}
}

func TestLoadManifestToleratesUnknownShapes(t *testing.T) {
	root := t.TempDir()
	// The "hooks": {} object shape appears in openai/plugins (superpowers).
	path := writeManifest(t, root, ".codex-plugin/plugin.json", `{
  "name": "superpowers",
  "version": "1.0.0",
  "description": "framework",
  "hooks": {},
  "skills": "./skills/"
}`)

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v, want tolerant parsing", err)
	}
	if len(manifest.Hooks) != 0 {
		t.Errorf("Hooks = %v, want ignored object shape", manifest.Hooks)
	}
	if len(manifest.Skills) != 1 || manifest.Skills[0] != "./skills/" {
		t.Errorf("Skills = %v, want [./skills/]", manifest.Skills)
	}
}

func TestLoadManifestIgnoresNonStringListEntries(t *testing.T) {
	root := t.TempDir()
	path := writeManifest(t, root, "clawy-plugin.json", `{
  "name": "mixed",
  "commands": ["./commands/", {"path": "./extra.md"}, 7]
}`)

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if len(manifest.Commands) != 1 || manifest.Commands[0] != "./commands/" {
		t.Errorf("Commands = %v, want only the string entry", manifest.Commands)
	}
}

func TestPathListUnmarshalForms(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"string", `"./skills/"`, []string{"./skills/"}},
		{"list", `["./a/", "./b/"]`, []string{"./a/", "./b/"}},
		{"null", `null`, nil},
		{"empty string", `"  "`, nil},
		{"object", `{}`, nil},
		{"number", `7`, nil},
		{"mixed list", `["./a/", 3]`, []string{"./a/"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var document struct {
				Skills pathList `json:"skills"`
			}
			payload := `{"skills": ` + tc.raw + `}`
			if err := json.Unmarshal([]byte(payload), &document); err != nil {
				t.Fatalf("Unmarshal(%s) error = %v", payload, err)
			}
			if len(document.Skills) != len(tc.want) {
				t.Fatalf("Skills = %v, want %v", []string(document.Skills), tc.want)
			}
			for i, want := range tc.want {
				if document.Skills[i] != want {
					t.Errorf("Skills[%d] = %q, want %q", i, document.Skills[i], want)
				}
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	pluginDir := t.TempDir()

	if got := resolvePath(pluginDir, "./skills/"); got != filepath.Join(pluginDir, "skills") {
		t.Errorf("relative path = %q", got)
	}

	absolute := filepath.Join(t.TempDir(), "abs", "path")
	if got := resolvePath(pluginDir, absolute); got != filepath.Clean(absolute) {
		t.Errorf("absolute path = %q, want %q", got, filepath.Clean(absolute))
	}
	if got := resolvePath(pluginDir, "  "); got != "" {
		t.Errorf("blank path = %q, want empty", got)
	}
}
