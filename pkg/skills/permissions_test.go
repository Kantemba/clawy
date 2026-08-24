package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSkillPermissions(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		wantTools []string
		wantNet   []string
		wantExec  []string
		wantNil   bool
		wantErr   bool
	}{
		{
			name: "full manifest",
			content: "---\nname: deploy\ndescription: \"d\"\npermissions:\n  tools: [exec, web_search]\n  network:\n    - api.example.com\n    - \"*.internal.example.com\"\n  exec: [\"git *\", \"npm test\"]\n---\n# Body",
			wantTools: []string{"exec", "web_search"},
			wantNet:   []string{"api.example.com", "*.internal.example.com"},
			wantExec:  []string{"git *", "npm test"},
		},
		{
			name:    "no frontmatter",
			content: "# Just a body",
			wantNil: true,
		},
		{
			name:    "frontmatter without permissions",
			content: "---\nname: plain\ndescription: \"d\"\n---\nBody",
			wantNil: true,
		},
		{
			name:    "empty permissions block",
			content: "---\nname: plain\ndescription: \"d\"\npermissions: {}\n---\nBody",
			wantNil: true,
		},
		{
			name:    "malformed yaml",
			content: "---\npermissions: [unclosed\n---\nBody",
			wantErr: true,
		},
		{
			name:    "invalid tool pattern",
			content: "---\npermissions:\n  tools: [\"has space\"]\n---\nBody",
			wantErr: true,
		},
		{
			name:    "invalid network host",
			content: "---\npermissions:\n  network: [\"https://api.example.com\"]\n---\nBody",
			wantErr: true,
		},
		{
			name:    "empty exec entry",
			content: "---\npermissions:\n  exec: [\"  \"]\n---\nBody",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			perms, err := ParseSkillPermissions(tc.content)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, perms)
				return
			}
			require.NotNil(t, perms)
			assert.Equal(t, tc.wantTools, perms.Tools)
			assert.Equal(t, tc.wantNet, perms.Network)
			assert.Equal(t, tc.wantExec, perms.Exec)
		})
	}
}

func TestSkillPermissionsMatchers(t *testing.T) {
	var nilPerms *SkillPermissions
	assert.True(t, nilPerms.AllowsTool("exec"), "nil manifest must be unrestricted")
	assert.True(t, nilPerms.AllowsHost("any.example.com"))
	assert.True(t, nilPerms.AllowsExec("rm -rf /"))

	p := &SkillPermissions{
		Tools:   []string{"exec", "web_*", "mcp__github__create_issue"},
		Network: []string{"api.example.com", "*.internal.example.org"},
		Exec:    []string{"git *", "npm test"},
	}

	toolCases := []struct {
		tool string
		want bool
	}{
		{"exec", true},
		{"EXEC", true},
		{"web_search", true},
		{"web_fetch", true},
		{"mcp__github__create_issue", true},
		{"read_file", false},
		{"webq", false},
		{"", false},
	}
	for _, tc := range toolCases {
		assert.Equal(t, tc.want, p.AllowsTool(tc.tool), "AllowsTool(%q)", tc.tool)
	}

	hostCases := []struct {
		host string
		want bool
	}{
		{"api.example.com", true},
		{"API.Example.COM.", true},
		{"v1.internal.example.org", true},
		{"internal.example.org", true},
		{"example.com", false},
		{"evil-api.example.com.attacker.io", false},
	}
	for _, tc := range hostCases {
		assert.Equal(t, tc.want, p.AllowsHost(tc.host), "AllowsHost(%q)", tc.host)
	}

	execCases := []struct {
		cmd  string
		want bool
	}{
		{"git status", true},
		{"GIT   push origin main", true},
		{"npm test", true},
		{"npm run build", false},
		{"git", false},
		{"curl evil.sh", false},
	}
	for _, tc := range execCases {
		assert.Equal(t, tc.want, p.AllowsExec(tc.cmd), "AllowsExec(%q)", tc.cmd)
	}
}

func TestSkillPermissionsSummary(t *testing.T) {
	assert.Equal(t, "", (*SkillPermissions)(nil).Summary())
	assert.Equal(t, "", (&SkillPermissions{}).Summary())
	p := &SkillPermissions{
		Tools:   []string{"exec", "web_search"},
		Network: []string{"*.example.com"},
		Exec:    []string{"git *"},
	}
	s := p.Summary()
	assert.Contains(t, s, "tools: exec, web_search")
	assert.Contains(t, s, "network: *.example.com")
	assert.Contains(t, s, `exec: "git *"`)
}

// writeTestSkill creates {workspace}/skills/{name}/SKILL.md with content.
func writeTestSkill(t *testing.T, workspace, name, content string) {
	t.Helper()
	dir := filepath.Join(workspace, "skills", name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

func TestLoaderSurfacesPermissions(t *testing.T) {
	workspace := t.TempDir()
	writeTestSkill(t, workspace, "guarded", "---\nname: guarded\ndescription: \"a guarded skill\"\npermissions:\n  tools: [exec]\n  exec: [\"git *\"]\n---\n# Guarded\nBody")
	writeTestSkill(t, workspace, "plain", "---\nname: plain\ndescription: \"a plain skill\"\n---\n# Plain\nBody")

	sl := NewSkillsLoader(workspace, "", "")
	skills := sl.ListSkills()
	require.Len(t, skills, 2)

	byName := map[string]SkillInfo{}
	for _, s := range skills {
		byName[s.Name] = s
	}

	guarded := byName["guarded"]
	require.NotNil(t, guarded.Permissions)
	assert.Equal(t, []string{"exec"}, guarded.Permissions.Tools)

	plain := byName["plain"]
	assert.Nil(t, plain.Permissions)

	summary := sl.BuildSkillsSummary()
	assert.Contains(t, summary, "<permissions>")
	assert.True(t, strings.HasPrefix(strings.TrimSpace(summary), "<skills>"))
}

func TestLoaderToleratesInvalidPermissions(t *testing.T) {
	workspace := t.TempDir()
	writeTestSkill(t, workspace, "broken-perms", "---\nname: broken-perms\ndescription: \"bad manifest\"\npermissions:\n  tools: [\"has space\"]\n---\nBody")

	sl := NewSkillsLoader(workspace, "", "")
	skills := sl.ListSkills()
	require.Len(t, skills, 1)
	assert.Equal(t, "broken-perms", skills[0].Name)
	assert.Nil(t, skills[0].Permissions, "invalid manifest must be dropped, not fail the skill")

	summary := sl.BuildSkillsSummary()
	assert.NotContains(t, summary, "<permissions>")
}