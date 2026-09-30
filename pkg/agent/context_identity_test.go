package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kantemba/clawy/pkg/identity"
)

func TestUserOwnedIdentityAndCache(t *testing.T) {
	workspace := t.TempDir()
	legacy := "Your name is Clawy. Keep the user's workspace rules."
	if err := os.WriteFile(filepath.Join(workspace, "AGENT.md"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	cb := NewContextBuilder(workspace)
	initial := cb.BuildSystemPromptWithCache()
	if !strings.Contains(initial, "You are Clawy") {
		t.Fatal("missing default identity")
	}
	profile := identity.Default()
	profile.Name = "Nova"
	profile.Role = "Creative partner"
	profile.Personality = "Playful and poetic"
	profile.Instructions = "Use Spanish"
	if err := identity.Save(workspace, profile); err != nil {
		t.Fatal(err)
	}
	prompt := cb.BuildSystemPromptWithCache()
	for _, expected := range []string{"You are Nova", "Creative partner", "Playful and poetic", "Use Spanish", "Ignore conflicting legacy names", "Keep the user's workspace rules", "## Important Rules"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("missing %q in prompt", expected)
		}
	}
	// Give edits a distinct timestamp even on coarse filesystem clocks.
	profile.Name = "Atlas"
	if err := identity.Save(workspace, profile); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(workspace, identity.FileName), future, future); err != nil {
		t.Fatal(err)
	}
	if prompt := cb.BuildSystemPromptWithCache(); !strings.Contains(prompt, "You are Atlas") || strings.Contains(prompt, "You are Nova") {
		t.Fatal("identity update did not invalidate cached system prompt")
	}
	if err := os.Remove(filepath.Join(workspace, identity.FileName)); err != nil {
		t.Fatal(err)
	}
	if prompt := cb.BuildSystemPromptWithCache(); !strings.Contains(prompt, "You are Clawy") {
		t.Fatal("identity deletion did not invalidate cache")
	}
}

func TestDiscoveryUsesSavedIdentity(t *testing.T) {
	workspace := t.TempDir()
	profile := identity.Default()
	profile.Name = "Nova"
	profile.Role = "Creative partner"
	if err := identity.Save(workspace, profile); err != nil {
		t.Fatal(err)
	}
	registry := &AgentRegistry{agents: map[string]*AgentInstance{
		"main": {ID: "main", Workspace: workspace},
	}}
	descriptor, ok := registry.GetAgentDescriptor("main")
	if !ok || descriptor.Name != "Nova" || descriptor.Description != "Creative partner" {
		t.Fatalf("agent descriptor = %+v, %v", descriptor, ok)
	}
	profile.Name = "Atlas"
	if err := identity.Save(workspace, profile); err != nil {
		t.Fatal(err)
	}
	descriptor, ok = registry.GetAgentDescriptor("main")
	if !ok || descriptor.Name != "Atlas" {
		t.Fatal("discovery identity did not refresh after save")
	}
}

func TestInvalidIdentityFallsBack(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, identity.FileName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if prompt := NewContextBuilder(workspace).BuildSystemPrompt(); !strings.Contains(prompt, "You are Clawy") {
		t.Fatal("corrupt identity broke the runtime")
	}
}
