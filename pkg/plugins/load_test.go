package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCommandFileScalarFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.md")
	content := "---\ndescription: Deploy a service\nargument-hint: \"<service> [env]\"\naliases: [ship, release]\n---\n\nDeploy {{args}} now.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	def, err := loadCommandFile(path)
	if err != nil {
		t.Fatalf("loadCommandFile() error = %v", err)
	}
	if def.Name != "deploy" {
		t.Errorf("Name = %q, want deploy", def.Name)
	}
	if def.Description != "Deploy a service" {
		t.Errorf("Description = %q", def.Description)
	}
	if def.Usage != "<service> [env]" {
		t.Errorf("Usage = %q", def.Usage)
	}
	if len(def.Aliases) != 2 {
		t.Errorf("Aliases = %v, want [ship release]", def.Aliases)
	}
	if def.Prompt != "Deploy {{args}} now." {
		t.Errorf("Prompt = %q", def.Prompt)
	}
}

// Real plugin repos write frontmatter for other tools: lists where a string is
// expected (argument-hint: [agent-description]) and keys we do not model.
func TestLoadCommandFileToleratesForeignFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build-agent.md")
	content := "---\ndescription: [Build an agent, then deploy it]\nargument-hint: [agent-description]\nallowed-tools: [Write, Bash]\n---\n\nBuild the agent {{args}}.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	def, err := loadCommandFile(path)
	if err != nil {
		t.Fatalf("loadCommandFile() error = %v, want tolerated frontmatter", err)
	}
	if def.Name != "build-agent" {
		t.Errorf("Name = %q, want build-agent", def.Name)
	}
	if def.Description != "Build an agent then deploy it" {
		t.Errorf("Description = %q, want the joined list", def.Description)
	}
	if def.Usage != "agent-description" {
		t.Errorf("Usage = %q, want agent-description", def.Usage)
	}
	if def.Prompt == "" {
		t.Error("Prompt is empty")
	}
}

func TestLoadCommandFileUnparseableFrontmatterStillLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.md")
	content := "---\n\tdescription: tab indented\n---\n\nStill useful.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	def, err := loadCommandFile(path)
	if err != nil {
		t.Fatalf("loadCommandFile() error = %v, want a fallback command", err)
	}
	if def.Name != "broken" {
		t.Errorf("Name = %q, want the file name fallback", def.Name)
	}
	if def.Description != "" {
		t.Errorf("Description = %q, want empty after an unusable block", def.Description)
	}
	if def.Prompt != "Still useful." {
		t.Errorf("Prompt = %q, want the body preserved", def.Prompt)
	}
}

func TestLoadCommandFileDerivesDescriptionFromBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connect-figma-components.md")
	content := "# /connect-figma-components\n\nCreate or update parserless Figma Code Connect template files for components.\n\n## Arguments\n\n- `figma_url`: required\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	def, err := loadCommandFile(path)
	if err != nil {
		t.Fatalf("loadCommandFile() error = %v", err)
	}
	if def.Description != "Create or update parserless Figma Code Connect template files for components." {
		t.Errorf("Description = %q, want the paragraph after the H1", def.Description)
	}
	if def.Name != "connect-figma-components" {
		t.Errorf("Name = %q", def.Name)
	}
}

func TestDeriveDescriptionEdgeCases(t *testing.T) {
	cases := map[string]string{
		"":                           "",
		"Just prose.\n":              "",
		"# Title\n":                  "",
		"# Title\n\n## Section\n":    "",
		"# Title\n\nDo the thing.":   "Do the thing.",
		"# T\n\nFirst line.\n\nMore": "First line.",
	}
	for input, want := range cases {
		if got := deriveDescription(input); got != want {
			t.Errorf("deriveDescription(%q) = %q, want %q", input, got, want)
		}
	}

	long := "# T\n\n" + strings.Repeat("word ", 100)
	if got := deriveDescription(long); len([]rune(got)) > 205 || !strings.HasSuffix(got, "…") {
		t.Errorf("long description should be truncated with an ellipsis, got %d runes: %q", len([]rune(got)), got)
	}
}

func TestLoadCommandFileRequiresBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(path, []byte("---\ndescription: nothing\n---\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := loadCommandFile(path); err == nil {
		t.Fatal("loadCommandFile() error = nil, want an empty-body error")
	}
}
