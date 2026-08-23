package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// SkillDirHasDefinition reports whether dir contains a readable SKILL.md with
// non-empty content.
func SkillDirHasDefinition(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) != ""
}

// SkillDirIsValid reports whether dir contains a SKILL.md whose parsed
// metadata satisfies the same validation rules the loader applies when
// listing skills (valid name, non-empty description). Installers use this as
// the post-download validity gate so a registry archive that is not actually
// a skill is rejected and rolled back instead of silently installed.
func SkillDirIsValid(dir string) bool {
	meta := (&SkillsLoader{}).getSkillMetadata(filepath.Join(dir, "SKILL.md"))
	if meta == nil {
		return false
	}
	if meta.Description == "" {
		return false
	}
	return ValidateSkillName(meta.Name) == nil
}