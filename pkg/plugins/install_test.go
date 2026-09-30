package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFromDirectoryContainingPlugins(t *testing.T) {
	cfg := testConfig(t)
	dest := filepath.Join(cfg.WorkspacePath(), "plugins")

	// A checkout that contains several plugins, like github.com/openai/plugins.
	checkout := t.TempDir()
	writeCodexPlugin(t, filepath.Join(checkout, "plugins", "figma"), "figma")
	writeCodexPlugin(t, filepath.Join(checkout, "plugins", "notion"), "notion")

	installed, err := Install(checkout, dest)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(installed) != 2 {
		t.Fatalf("Install() installed %d plugins, want 2", len(installed))
	}

	bundle := Discover(cfg)
	if len(bundle.Plugins) != 2 {
		t.Fatalf("Discover() after install found %d plugins, want 2 (%v)", len(bundle.Plugins), bundle.Warnings)
	}
}

func TestInstallSinglePluginDirectory(t *testing.T) {
	cfg := testConfig(t)
	dest := filepath.Join(cfg.WorkspacePath(), "plugins")

	source := writeCodexPlugin(t, filepath.Join(t.TempDir(), "notion"), "notion")

	installed, err := Install(source, dest)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("Install() installed %d plugins, want 1", len(installed))
	}
	if _, err := os.Stat(filepath.Join(installed[0], ".codex-plugin", "plugin.json")); err != nil {
		t.Errorf("manifest was not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installed[0], "skills", "notion-skill", "SKILL.md")); err != nil {
		t.Errorf("skill was not copied: %v", err)
	}
}

func TestInstallRefusesOverwrite(t *testing.T) {
	cfg := testConfig(t)
	dest := filepath.Join(cfg.WorkspacePath(), "plugins")
	source := writeCodexPlugin(t, filepath.Join(t.TempDir(), "notion"), "notion")

	if _, err := Install(source, dest); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if _, err := Install(source, dest); err == nil {
		t.Fatal("second Install() error = nil, want already-installed error")
	}
}

func TestInstallRejectsSourceWithoutManifest(t *testing.T) {
	if _, err := Install(t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("Install() error = nil, want no-manifest error")
	}
}

func TestScaffoldCreatesDiscoverablePlugin(t *testing.T) {
	cfg := testConfig(t)
	dest := filepath.Join(cfg.WorkspacePath(), "plugins")

	result, err := Scaffold(dest, "my-plugin", "My demo plugin")
	if err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}
	if result.Dir != filepath.Join(dest, "my-plugin") {
		t.Errorf("Dir = %q", result.Dir)
	}
	if len(result.Files) != 4 {
		t.Errorf("Files = %v, want 4 entries", result.Files)
	}

	// A second scaffold over the same directory must not clobber it.
	if _, err := Scaffold(dest, "my-plugin", "again"); err == nil {
		t.Error("Scaffold() on an existing plugin should fail")
	}

	bundle := Discover(cfg)
	if len(bundle.Plugins) != 1 {
		t.Fatalf("Discover() found %d plugins, want the scaffolded one (%v)", len(bundle.Plugins), bundle.Warnings)
	}

	plugin := bundle.Plugins[0]
	if plugin.Name() != "my-plugin" {
		t.Errorf("Name() = %q, want my-plugin", plugin.Name())
	}
	if len(plugin.SkillRoots) != 1 {
		t.Errorf("SkillRoots = %v, want the scaffolded skills dir", plugin.SkillRoots)
	}
	if len(plugin.Commands) != 1 {
		t.Fatalf("Commands = %+v, want one prompt command", plugin.Commands)
	}
	if plugin.Commands[0].Name != "my-plugin" {
		t.Errorf("command name = %q, want my-plugin", plugin.Commands[0].Name)
	}
}

func TestScaffoldRejectsInvalidName(t *testing.T) {
	if _, err := Scaffold(t.TempDir(), "not a name", ""); err == nil {
		t.Error("Scaffold() error = nil, want invalid-name error")
	}
}

func TestBundleDescribeAndCounts(t *testing.T) {
	cfg := testConfig(t)
	writeCodexPlugin(t, filepath.Join(cfg.WorkspacePath(), "plugins", "notion"), "notion")

	bundle := Discover(cfg)
	commands, servers, hooks := bundle.Counts()
	if commands != 1 || servers != 1 || hooks != 1 {
		t.Errorf("Counts() = %d, %d, %d; want 1, 1, 1", commands, servers, hooks)
	}
	if bundle.Describe() == "no plugins" {
		t.Error("Describe() should summarize a non-empty bundle")
	}

	empty := &Bundle{}
	if empty.Describe() != "no plugins" {
		t.Errorf("Describe() = %q", empty.Describe())
	}
	if _, ok := empty.Find("x"); ok {
		t.Error("Find() on an empty bundle should miss")
	}
}
