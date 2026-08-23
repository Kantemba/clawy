package skills

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Kantemba/clawy/pkg/config"
)

// GlobalSkillsDir returns the global, user-wide skills directory
// (<clawy home>/skills, typically ~/.clawy/skills).
func GlobalSkillsDir() string {
	return filepath.Join(config.GetHome(), "skills")
}

// DefaultBuiltinSkillsDir returns the default builtin skills root: the
// "skills" directory under the current working directory. It mirrors the
// lookup behavior historically used by the agent context builder and the
// web backend.
func DefaultBuiltinSkillsDir() string {
	wd, err := os.Getwd()
	if err != nil {
		// Extremely rare; fall back to a relative path so lookups behave
		// consistently with previous versions.
		return "skills"
	}
	return filepath.Join(wd, "skills")
}

// ResolveBuiltinSkillsDir resolves the effective builtin skills directory,
// honoring the CLAWY_BUILTIN_SKILLS environment override and falling back to
// the provided default when unset. All entry points (agent, CLI, web backend)
// must go through this helper so every component agrees on where built-in
// skills live.
func ResolveBuiltinSkillsDir(fallback string) string {
	if override := strings.TrimSpace(os.Getenv(config.EnvBuiltinSkills)); override != "" {
		return override
	}
	return fallback
}