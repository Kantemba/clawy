package skills

import (
	"path/filepath"
	"sync"
)

// Plugin skill roots are contributed by the plugin loader (pkg/plugins).
// They are process-wide rather than per-loader: a plugin directory is not
// tied to one agent workspace, and every loader (agent, CLI, web backend,
// evolution) must agree on which skills a plugin provides.
var (
	pluginRootsMu sync.RWMutex
	pluginRoots   []string
)

// SetPluginSkillRoots replaces the process-wide plugin skill roots. Passing
// nil clears them (plugins disabled).
func SetPluginSkillRoots(roots []string) {
	cleaned := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		trimmed := filepath.Clean(root)
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}

	pluginRootsMu.Lock()
	pluginRoots = cleaned
	pluginRootsMu.Unlock()
}

// PluginSkillRoots returns a copy of the current plugin skill roots.
func PluginSkillRoots() []string {
	pluginRootsMu.RLock()
	defer pluginRootsMu.RUnlock()
	if len(pluginRoots) == 0 {
		return nil
	}
	out := make([]string, len(pluginRoots))
	copy(out, pluginRoots)
	return out
}
