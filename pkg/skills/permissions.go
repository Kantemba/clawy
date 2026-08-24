package skills

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillPermissions declares the capabilities a skill expects to use. It is a
// disclosure manifest written in the SKILL.md frontmatter:
//
//	---
//		name: deploy-helper
//		description: "Deploys the staging site"
//		permissions:
//			tools: [exec, web_search]
//			network: ["*.internal.example.com"]
//			exec: ["git *", "npm test"]
//	---
//
// Installers surface it for human review, and the skill catalog exposes it to
// the model. The matching helpers treat a nil manifest as unrestricted; once
// a manifest is declared, only listed entries match. Patterns support a
// trailing "*" wildcard for prefix matching (tools/exec) and a leading "*."
// wildcard for subdomain matching (network).
type SkillPermissions struct {
	Tools   []string `yaml:"tools,omitempty"   json:"tools,omitempty"`
	Network []string `yaml:"network,omitempty" json:"network,omitempty"`
	Exec    []string `yaml:"exec,omitempty"    json:"exec,omitempty"`
}

const (
	maxPermissionEntries       = 32
	maxPermissionPatternLength = 256
)

var (
	permissionToolPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.:-]*\*?$`)
	permissionHostPattern = regexp.MustCompile(`^(\*\.)?[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)
	permissionControlRe   = regexp.MustCompile(`[[:cntrl:]]`)
)

// ParseSkillPermissions extracts and validates the optional `permissions`
// block from full SKILL.md content. Returns (nil, nil) when no manifest is
// declared.
func ParseSkillPermissions(skillContent string) (*SkillPermissions, error) {
	frontmatter, _ := splitFrontmatter(skillContent)
	if frontmatter == "" {
		return nil, nil
	}
	return parsePermissionsFrontmatter(frontmatter)
}

func parsePermissionsFrontmatter(frontmatter string) (*SkillPermissions, error) {
	var wrapper struct {
		Permissions *SkillPermissions `yaml:"permissions"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &wrapper); err != nil {
		return nil, fmt.Errorf("invalid permissions frontmatter: %w", err)
	}
	p := wrapper.Permissions
	if p == nil || p.Empty() {
		return nil, nil
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// Empty reports whether the manifest declares nothing.
func (p *SkillPermissions) Empty() bool {
	return p == nil || (len(p.Tools) == 0 && len(p.Network) == 0 && len(p.Exec) == 0)
}

// Validate checks entry counts, lengths, and pattern shapes.
func (p *SkillPermissions) Validate() error {
	if p == nil {
		return nil
	}
	var errs error
	errs = errors.Join(errs, validatePermissionList("tools", p.Tools, permissionToolPattern,
		"must be an identifier with optional trailing '*'"))
	errs = errors.Join(errs, validatePermissionList("network", p.Network, permissionHostPattern,
		"must be a hostname like 'api.example.com' or wildcard '*.example.com'"))
	for i, e := range p.Exec {
		entry := strings.TrimSpace(e)
		switch {
		case entry == "":
			errs = errors.Join(errs, fmt.Errorf("permissions.exec[%d] must not be empty", i))
		case len(entry) > maxPermissionPatternLength:
			errs = errors.Join(errs, fmt.Errorf("permissions.exec[%d] exceeds %d characters", i, maxPermissionPatternLength))
		case permissionControlRe.MatchString(entry):
			errs = errors.Join(errs, fmt.Errorf("permissions.exec[%d] must not contain control characters", i))
		}
	}
	if len(p.Exec) > maxPermissionEntries {
		errs = errors.Join(errs, fmt.Errorf("permissions.exec exceeds %d entries", maxPermissionEntries))
	}
	return errs
}

func validatePermissionList(field string, entries []string, pattern *regexp.Regexp, hint string) error {
	if len(entries) > maxPermissionEntries {
		return fmt.Errorf("permissions.%s exceeds %d entries", field, maxPermissionEntries)
	}
	var errs error
	for i, raw := range entries {
		entry := strings.TrimSpace(raw)
		switch {
		case entry == "":
			errs = errors.Join(errs, fmt.Errorf("permissions.%s[%d] must not be empty", field, i))
		case len(entry) > maxPermissionPatternLength:
			errs = errors.Join(errs, fmt.Errorf("permissions.%s[%d] exceeds %d characters", field, i, maxPermissionPatternLength))
		case !pattern.MatchString(entry):
			errs = errors.Join(errs, fmt.Errorf("permissions.%s[%d] %q is invalid: %s", field, i, entry, hint))
		}
	}
	return errs
}

// Summary renders a compact one-line description of the manifest, e.g.
// `tools: exec, web; network: *.example.com; exec: "git *"`.
func (p *SkillPermissions) Summary() string {
	if p.Empty() {
		return ""
	}
	var parts []string
	if len(p.Tools) > 0 {
		parts = append(parts, "tools: "+strings.Join(p.Tools, ", "))
	}
	if len(p.Network) > 0 {
		parts = append(parts, "network: "+strings.Join(p.Network, ", "))
	}
	if len(p.Exec) > 0 {
		quoted := make([]string, len(p.Exec))
		for i, e := range p.Exec {
			quoted[i] = strconv.Quote(e)
		}
		parts = append(parts, "exec: "+strings.Join(quoted, ", "))
	}
	return strings.Join(parts, "; ")
}

// AllowsTool reports whether the given tool name is declared. A nil or empty
// manifest is unrestricted.
func (p *SkillPermissions) AllowsTool(tool string) bool {
	return p == nil || matchWildcard(p.Tools, strings.ToLower(strings.TrimSpace(tool)))
}

// AllowsHost reports whether the given host matches a declared network
// pattern. "*.example.com" matches subdomains of example.com and its apex;
// an exact pattern matches only itself. Comparison is case-insensitive.
func (p *SkillPermissions) AllowsHost(host string) bool {
	if p == nil {
		return true
	}
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimSuffix(host, ".")
	for _, raw := range p.Network {
		pattern := strings.ToLower(strings.TrimSpace(raw))
		wildcard := false
		if rest, ok := strings.CutPrefix(pattern, "*."); ok {
			pattern = rest
			wildcard = true
		}
		if host == pattern || (wildcard && strings.HasSuffix(host, "."+pattern)) {
			return true
		}
	}
	return false
}

// AllowsExec reports whether the given command line matches a declared exec
// pattern. Patterns are matched against the lower-cased command prefix; a
// trailing "*" allows any arguments after the fixed part.
func (p *SkillPermissions) AllowsExec(command string) bool {
	if p == nil {
		return true
	}
	cmd := strings.ToLower(strings.Join(strings.Fields(command), " "))
	for _, raw := range p.Exec {
		pattern := strings.ToLower(strings.Join(strings.Fields(raw), " "))
		base, wildcard := strings.CutSuffix(pattern, "*")
		if cmd == base || (wildcard && strings.HasPrefix(cmd, base)) {
			return true
		}
	}
	return false
}

// matchWildcard returns true when value equals an entry exactly or, for
// entries ending in "*", has the entry's base as a prefix.
func matchWildcard(entries []string, value string) bool {
	if value == "" {
		return false
	}
	for _, raw := range entries {
		entry := strings.ToLower(strings.TrimSpace(raw))
		base, wildcard := strings.CutSuffix(entry, "*")
		if value == base || (wildcard && strings.HasPrefix(value, base)) {
			return true
		}
	}
	return false
}
