package utils

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	clawy "github.com/Kantemba/clawy"
	"github.com/Kantemba/clawy/pkg/config"
)

// EnsureOnboarded bootstraps first-run files without invoking interactive CLI
// onboarding. Existing configurations and workspace files are never reset.
func EnsureOnboarded(configPath string) error {
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config: %w", err)
	}

	cfg := config.DefaultConfig()
	workspace := cfg.WorkspacePath()
	if err := fs.WalkDir(clawy.OnboardWorkspace, "workspace", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("workspace", path)
		if err != nil {
			return err
		}
		if rel == "AGENTS.md" || rel == "IDENTITY.md" {
			return nil
		}
		target := filepath.Join(workspace, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		data, err := clawy.OnboardWorkspace.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}); err != nil {
		return fmt.Errorf("initialize workspace: %w", err)
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("initialize config: %w", err)
	}
	return nil
}
