// Package identity stores a user-owned agent identity in its workspace.
package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Kantemba/clawy/pkg/fileutil"
)

const FileName = "IDENTITY.json"

type Profile struct {
	Name         string `json:"name"`
	Avatar       string `json:"avatar"`
	Role         string `json:"role"`
	Personality  string `json:"personality"`
	Instructions string `json:"instructions"`
	Greeting     string `json:"greeting"`
	Configured   bool   `json:"configured"`
}

func Default() Profile {
	return Profile{Name: "Clawy", Avatar: "🦞", Role: "Personal AI assistant", Personality: "Calm, helpful, and practical.", Greeting: "How can I help you today?"}
}

// Normalize validates bounded text, not file paths or executable configuration.
func (p *Profile) Normalize() error {
	fields := []struct {
		name       string
		value      *string
		max        int
		singleLine bool
	}{
		{"name", &p.Name, 64, true},
		{"avatar", &p.Avatar, 16, true},
		{"role", &p.Role, 200, true},
		{"personality", &p.Personality, 4000, false},
		{"instructions", &p.Instructions, 8000, false},
		{"greeting", &p.Greeting, 300, true},
	}
	for _, field := range fields {
		*field.value = strings.TrimSpace(*field.value)
		if !utf8.ValidString(*field.value) || utf8.RuneCountInString(*field.value) > field.max {
			return fmt.Errorf("%s must be valid text of at most %d characters", field.name, field.max)
		}
		for _, r := range *field.value {
			if unicode.IsControl(r) && (field.singleLine || (r != '\n' && r != '\r' && r != '\t')) {
				return fmt.Errorf("%s contains unsupported control characters", field.name)
			}
		}
	}
	if p.Name == "" {
		return errors.New("name is required")
	}
	if p.Avatar == "" {
		p.Avatar = "🦞"
	}
	return nil
}

func Load(workspace string) (Profile, error) {
	data, err := os.ReadFile(filepath.Join(workspace, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return Profile{}, err
	}
	if err := p.Normalize(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Save commits the entire profile atomically; workspace instructions are untouched.
func Save(workspace string, p Profile) error {
	if err := p.Normalize(); err != nil {
		return err
	}
	p.Configured = true
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(filepath.Join(workspace, FileName), append(data, '\n'), 0o600)
}

// Prompt supersedes only legacy identity/style, not permissions or runtime rules.
func (p Profile) Prompt() string {
	if !p.Configured {
		return ""
	}
	return fmt.Sprintf(`## User-owned identity

Your name is %s. You are an AI agent created by your user on the Clawy platform.
This profile is authoritative for your name, role, and communication style. Ignore conflicting legacy names or personality defaults in AGENT.md, AGENTS.md, SOUL.md, and IDENTITY.md. Runtime safety, tool permissions, and current user instructions still apply.

Role: %s

Personality and communication style:
%s

User's custom instructions:
%s
`, p.Name, p.Role, p.Personality, p.Instructions)
}
