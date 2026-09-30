package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkillFixture(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n\nBody of " + name + ".\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func TestPluginSkillRootsOrderAndSources(t *testing.T) {
	t.Cleanup(func() { SetPluginSkillRoots(nil) })

	workspace := t.TempDir()
	global := t.TempDir()
	builtin := t.TempDir()
	pluginRoot := t.TempDir()

	writeSkillFixture(t, filepath.Join(workspace, "skills"), "shared", "workspace wins")
	writeSkillFixture(t, pluginRoot, "shared", "plugin loses")
	writeSkillFixture(t, pluginRoot, "plugin-only", "only in a plugin")

	SetPluginSkillRoots([]string{pluginRoot})
	loader := NewSkillsLoader(workspace, global, builtin)

	roots := loader.SkillRoots()
	if len(roots) != 4 {
		t.Fatalf("SkillRoots() = %v, want 4 roots", roots)
	}
	if roots[0] != filepath.Join(workspace, "skills") {
		t.Errorf("roots[0] = %q, want workspace skills", roots[0])
	}
	if roots[1] != global {
		t.Errorf("roots[1] = %q, want global skills", roots[1])
	}
	if roots[2] != filepath.Clean(pluginRoot) {
		t.Errorf("roots[2] = %q, want the plugin root", roots[2])
	}
	if roots[3] != builtin {
		t.Errorf("roots[3] = %q, want builtin skills", roots[3])
	}

	infos := loader.ListSkills()
	sources := map[string]string{}
	for _, info := range infos {
		sources[info.Name] = info.Source
	}

	if sources["shared"] != "workspace" {
		t.Errorf("shared source = %q, want workspace (user files beat plugins)", sources["shared"])
	}
	if sources["plugin-only"] != "plugin" {
		t.Errorf("plugin-only source = %q, want plugin", sources["plugin-only"])
	}

	content, ok := loader.LoadSkill("plugin-only")
	if !ok {
		t.Fatal("LoadSkill(plugin-only) missed the plugin root")
	}
	if !strings.Contains(content, "Body of plugin-only.") {
		t.Errorf("LoadSkill() = %q, want the skill body", content)
	}

	summary := loader.BuildSkillsSummary()
	if !strings.Contains(summary, "<source>plugin</source>") {
		t.Errorf("skill catalog does not label plugin skills:\n%s", summary)
	}
}

func TestSetPluginSkillRootsDeduplicates(t *testing.T) {
	t.Cleanup(func() { SetPluginSkillRoots(nil) })

	root := t.TempDir()
	SetPluginSkillRoots([]string{root, root + string(filepath.Separator)})

	got := PluginSkillRoots()
	if len(got) != 1 {
		t.Errorf("PluginSkillRoots() = %v, want a single deduplicated root", got)
	}

	// Returned slice must be a copy: mutating it cannot corrupt the registry.
	got[0] = "mutated"
	if again := PluginSkillRoots(); again[0] == "mutated" {
		t.Error("PluginSkillRoots() returned the internal slice")
	}

	SetPluginSkillRoots(nil)
	if roots := PluginSkillRoots(); roots != nil {
		t.Errorf("PluginSkillRoots() = %v after clear, want nil", roots)
	}
}
