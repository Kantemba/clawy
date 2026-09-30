package plugins

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Install copies plugins into destRoot and returns the installed plugin
// directories.
//
// source may be:
//
//   - a plugin directory (a directory carrying a manifest), copied as one plugin;
//   - a directory that contains plugins — for example a clone of
//     https://github.com/openai/plugins — in which case every plugin found
//     inside is installed;
//   - a git URL (https://… , git@…:… , … .git), cloned into a temporary
//     directory first. This requires `git` on PATH.
func Install(source, destRoot string) ([]string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("plugin source is required")
	}
	if destRoot == "" {
		return nil, fmt.Errorf("destination directory is required")
	}

	cleanup := func() {}
	if isGitURL(source) {
		cloneDir, err := cloneGitSource(source)
		if err != nil {
			return nil, err
		}
		cleanup = func() { _ = os.RemoveAll(cloneDir) }
		source = cloneDir
	}
	defer cleanup()

	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("plugin source %s: %w", source, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("plugin source %s is not a directory", source)
	}

	dirs := pluginDirsIn(source)
	if len(dirs) == 0 {
		return nil, fmt.Errorf("no plugin manifest found under %s (expected .codex-plugin/plugin.json or clawy-plugin.json)", source)
	}

	var installed []string
	var failures []string
	for _, dir := range dirs {
		manifest, err := LoadManifest(FindManifest(dir))
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		dest := filepath.Join(destRoot, manifest.Name)
		if _, err := os.Stat(dest); err == nil {
			failures = append(failures, fmt.Sprintf("plugin %q is already installed at %s", manifest.Name, dest))
			continue
		}
		if err := CopyDir(dir, dest); err != nil {
			failures = append(failures, fmt.Sprintf("install plugin %q: %v", manifest.Name, err))
			continue
		}
		installed = append(installed, dest)
	}

	// A broken plugin in a repository must not block the healthy ones.
	if len(failures) > 0 {
		return installed, fmt.Errorf("skipped %d entr%s:\n  %s",
			len(failures), pluralSuffix(len(failures)), strings.Join(failures, "\n  "))
	}

	return installed, nil
}

func pluralSuffix(count int) string {
	if count == 1 {
		return "y"
	}
	return "ies"
}

// pluginDirsIn returns the plugin directories at or below root.
func pluginDirsIn(root string) []string {
	if FindManifest(root) != "" {
		return []string{root}
	}

	var dirs []string
	for _, candidate := range candidateDirs(root) {
		if candidate == root {
			continue
		}
		if FindManifest(candidate) != "" {
			dirs = append(dirs, candidate)
		}
	}
	return dirs
}

// isGitURL reports whether source should be cloned instead of copied.
func isGitURL(source string) bool {
	if strings.HasSuffix(source, ".git") {
		return true
	}
	if strings.HasPrefix(source, "git@") {
		return true
	}
	for _, scheme := range []string{"https://", "http://", "ssh://", "git://"} {
		if strings.HasPrefix(source, scheme) {
			return true
		}
	}
	return false
}

func cloneGitSource(url string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("installing from a git URL requires git: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "clawy-plugin-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	clone := exec.Command("git", "clone", "--depth", "1", url, tmpDir)
	clone.Stdout = nil
	clone.Stderr = nil
	if err := clone.Run(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("git clone %s failed: %w", url, err)
	}
	return tmpDir, nil
}

// CopyDir recursively copies src into dst, preserving the executable bit and
// skipping VCS metadata.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relative, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, relative)
		if relative == "." {
			return os.MkdirAll(target, 0o755)
		}

		// Never carry VCS metadata (or nested plugin checkouts) along.
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == ".hg" || entry.Name() == ".svn") {
			return filepath.SkipDir
		}

		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// FileExists reports whether path exists and is not a directory.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
