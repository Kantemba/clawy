package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kantemba/clawy/pkg/config"
)

func TestEnsureOnboardedSkipsWhenConfigExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvHome, home)
	path := filepath.Join(home, "config.json")
	const original = `{"custom":"preserve me"}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureOnboarded(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != original {
		t.Fatalf("existing config changed: %s, %v", data, err)
	}
	if _, err := os.Stat(config.DefaultConfig().WorkspacePath()); !os.IsNotExist(err) {
		t.Fatalf("workspace unexpectedly initialized: %v", err)
	}
}

func TestEnsureOnboardedBootstrapsWithoutCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvHome, home)
	t.Setenv(config.EnvBinary, filepath.Join(home, "nonexistent-clawy"))
	path := filepath.Join(home, "custom", "config.json")
	if err := EnsureOnboarded(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SOUL.md", "USER.md", "memory/MEMORY.md", "skills/weather/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(cfg.WorkspacePath(), name)); err != nil {
			t.Fatalf("missing template %s: %v", name, err)
		}
	}
	if err := EnsureOnboarded(path); err != nil {
		t.Fatalf("second run: %v", err)
	}
}

func TestEnsureOnboardedPreservesExistingWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvHome, home)
	workspace := config.DefaultConfig().WorkspacePath()
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, "SOUL.md")
	if err := os.WriteFile(path, []byte("my custom assistant"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureOnboarded(filepath.Join(home, "config.json")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "my custom assistant" {
		t.Fatalf("existing workspace file changed: %s, %v", data, err)
	}
}

func TestEnsureOnboardedReportsFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvHome, home)
	blocked := filepath.Join(home, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureOnboarded(filepath.Join(blocked, "config.json")); err == nil {
		t.Fatal("expected initialization error")
	}
}
