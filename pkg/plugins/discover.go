package plugins

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/logger"
)

// DestinationRoot returns the directory `clawy plugin install` and
// `clawy plugin new` write to: <workspace>/plugins.
func DestinationRoot(cfg *config.Config) string {
	if cfg == nil {
		return filepath.Join(config.GetHome(), "plugins")
	}
	if workspace := cfg.WorkspacePath(); workspace != "" {
		return filepath.Join(workspace, "plugins")
	}
	return filepath.Join(config.GetHome(), "plugins")
}

// SearchRoots returns the directories that are scanned for plugins, in
// priority order:
//
//  1. $CLAWY_PLUGINS (os.PathListSeparator separated)
//  2. config plugins.dirs
//  3. <workspace>/plugins — where `clawy plugin new` creates plugins
//  4. ~/.clawy/plugins
//
// Duplicate roots are removed while keeping the first (highest priority)
// occurrence.
func SearchRoots(cfg *config.Config) []string {
	var roots []string
	seen := map[string]struct{}{}

	add := func(entry string) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return
		}
		if entry == "~" || strings.HasPrefix(entry, "~/") || strings.HasPrefix(entry, `~\`) {
			if home, err := os.UserHomeDir(); err == nil && home != "" {
				entry = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(entry, "~"), `/\`))
			}
		}
		absolute, err := filepath.Abs(entry)
		if err != nil {
			absolute = filepath.Clean(entry)
		}
		if _, exists := seen[absolute]; exists {
			return
		}
		seen[absolute] = struct{}{}
		roots = append(roots, absolute)
	}

	if raw := os.Getenv(config.EnvPlugins); raw != "" {
		for _, entry := range filepath.SplitList(raw) {
			add(entry)
		}
	}
	if cfg != nil {
		for _, entry := range cfg.Plugins.Dirs {
			add(entry)
		}
		if workspace := cfg.WorkspacePath(); workspace != "" {
			add(filepath.Join(workspace, "plugins"))
		}
	}
	add(filepath.Join(config.GetHome(), "plugins"))

	return roots
}

// Discover loads every plugin reachable from the configured search roots.
// It is read-only: nothing is merged into cfg and no global state changes.
func Discover(cfg *config.Config) *Bundle {
	bundle := &Bundle{}
	if cfg == nil || !cfg.Plugins.Enabled {
		return bundle
	}

	disabled := cfg.Plugins.EffectiveDisabled()
	seenDirs := map[string]struct{}{}
	seenNames := map[string]string{}

	for _, root := range SearchRoots(cfg) {
		for _, dir := range candidateDirs(root) {
			manifestPath := FindManifest(dir)
			if manifestPath == "" {
				continue
			}
			key := canonicalDir(dir)
			if _, duplicate := seenDirs[key]; duplicate {
				continue
			}
			seenDirs[key] = struct{}{}

			plugin, warnings, err := loadPlugin(manifestPath, disabled)
			if err != nil {
				bundle.Warnings = append(bundle.Warnings, err.Error())
				logger.WarnCF("plugins", "Skipping plugin",
					map[string]any{"manifest": manifestPath, "error": err.Error()})
				continue
			}
			if plugin == nil {
				continue
			}

			if firstDir, duplicate := seenNames[strings.ToLower(plugin.Name())]; duplicate {
				bundle.Warnings = append(bundle.Warnings,
					"plugin "+plugin.Name()+" in "+plugin.Dir+" ignored: already loaded from "+firstDir)
				continue
			}
			seenNames[strings.ToLower(plugin.Name())] = plugin.Dir

			bundle.Plugins = append(bundle.Plugins, plugin)
			bundle.Warnings = append(bundle.Warnings, warnings...)
		}
	}

	sort.SliceStable(bundle.Plugins, func(i, j int) bool {
		return strings.ToLower(bundle.Plugins[i].Name()) < strings.ToLower(bundle.Plugins[j].Name())
	})
	bundle.finish()
	return bundle
}

// candidateDirs lists the directories probed inside a search root: the root
// itself, each of its children, and — for repository checkouts such as
// github.com/openai/plugins — the children of a nested plugins/ directory at
// either of those levels. That covers both "clone used as the search root"
// and "clone dropped into ~/.clawy/plugins/".
func candidateDirs(root string) []string {
	root = filepath.Clean(root)
	dirs := []string{root}

	entries, err := os.ReadDir(root)
	if err != nil {
		return dirs
	}
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}

	for _, base := range dirs {
		nested := filepath.Join(base, "plugins")
		if nestedEntries, err := os.ReadDir(nested); err == nil {
			for _, entry := range nestedEntries {
				if entry.IsDir() {
					dirs = append(dirs, filepath.Join(nested, entry.Name()))
				}
			}
		}
	}

	return dirs
}

func canonicalDir(dir string) string {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	return filepath.Clean(absolute)
}
