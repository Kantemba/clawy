package evolution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kantemba/clawy/pkg/memory"
	"github.com/Kantemba/clawy/pkg/providers"
	"github.com/Kantemba/clawy/pkg/skills"
)

// testIdentityProvider is a stub providers.LLMProvider that returns a canned
// response and records how many Chat calls it received.
type testIdentityProvider struct {
	response     *providers.LLMResponse
	err          error
	defaultModel string
	lastModel    string
	chatCalls    int
	lastUser     string
}

func (p *testIdentityProvider) Chat(
	_ context.Context,
	messages []providers.Message,
	_ []providers.ToolDefinition,
	model string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	p.chatCalls++
	p.lastModel = model
	for _, m := range messages {
		if m.Role == "user" {
			p.lastUser = m.Content
		}
	}
	return p.response, p.err
}

func (p *testIdentityProvider) GetDefaultModel() string { return p.defaultModel }

func identityCuratorFor(t *testing.T, provider providers.LLMProvider, workspace string) *LLMIdentityCurator {
	t.Helper()
	r := NewLLMIdentityCurator(workspace, provider, "test-model")
	r.cooldown = 5 * time.Minute
	r.now = func() time.Time { return time.Now() }
	return r
}

func writeTestIdentityFile(t *testing.T, workspace, name, content string) string {
	t.Helper()
	path := filepath.Join(workspace, name)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", workspace, err)
	}
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return path
}

func TestParseIdentityFacts_ParsesCleanJSON(t *testing.T) {
	facts, ok := parseIdentityFacts(`{"soul":["Stay precise.","Prefer native-name lookup."],"user":["Prefers terse answers."]}`)
	if !ok {
		t.Fatal("expected ok")
	}
	if len(facts.Soul) != 2 || facts.Soul[0] != "Stay precise." {
		t.Fatalf("soul = %#v", facts.Soul)
	}
	if len(facts.User) != 1 || facts.User[0] != "Prefers terse answers." {
		t.Fatalf("user = %#v", facts.User)
	}
}

func TestParseIdentityFacts_StripsMarkdownFences(t *testing.T) {
	facts, ok := parseIdentityFacts("```json\n{\"soul\":[\"x\"],\"user\":[\"y\"]}\n```\n")
	if !ok {
		t.Fatal("expected ok after fence stripping")
	}
	if len(facts.Soul) != 1 || len(facts.User) != 1 {
		t.Fatalf("got %#v", facts)
	}
}

func TestParseIdentityFacts_RejectsGarbage(t *testing.T) {
	if _, ok := parseIdentityFacts("not json at all"); ok {
		t.Fatal("expected ok=false for garbage")
	}
}

func TestApplyFacts_AppendsToNewFile(t *testing.T) {
	out, changed := applyFacts("", []string{"Stay precise.", "Be terse."}, 3)
	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.Contains(out, identitySectionBegin) || !strings.Contains(out, identitySectionEnd) {
		t.Fatalf("missing section markers:\n%s", out)
	}
	if !strings.Contains(out, "- Stay precise.\n") || !strings.Contains(out, "- Be terse.") {
		t.Fatalf("missing bullets:\n%s", out)
	}
}

func TestApplyFacts_DedupsExactRepeat(t *testing.T) {
	existing := "# Soul\n" + identitySectionBegin + "\n- Stay precise.\n" + identitySectionEnd + "\n"
	out, changed := applyFacts(existing, []string{"Stay precise.", "Be terse."}, 3)
	if !changed {
		t.Fatal("expected the non-duplicate fact to change the file")
	}
	// "Stay precise." already present -> skipped; "Be terse." appended.
	before := strings.Count(out, "- Stay precise.")
	if before != 1 {
		t.Fatalf("expected exactly one copy of the duplicate fact, got %d", before)
	}
	if !strings.Contains(out, "- Be terse.") {
		t.Fatalf("new fact not appended:\n%s", out)
	}
}

func TestApplyFacts_DedupsSuperset(t *testing.T) {
	existing := identitySectionBegin + "\n- Be helpful.\n" + identitySectionEnd + "\n"
	_, changed := applyFacts(existing, []string{"Be helpful and kind."}, 3)
	if changed {
		t.Fatal("expected superset of an existing fact to be treated as a duplicate")
	}
}

func TestApplyFacts_CapsAtMax(t *testing.T) {
	newFacts := []string{"a", "b", "c", "d", "e"}
	_, changed := applyFacts("", newFacts, 2)
	if !changed {
		t.Fatal("expected changed=true")
	}
}

func TestApplyFacts_SplicesPreservingSurroundingContent(t *testing.T) {
	original := "# Soul\n\nStay sharp.\n\n" +
		identitySectionBegin + "\n- old fact\n" + identitySectionEnd + "\n\n" +
		"## Notes\nSee below.\n"
	out, changed := applyFacts(original, []string{"new fact"}, 3)
	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.Contains(out, "Stay sharp.") || !strings.Contains(out, "## Notes\nSee below.") {
		t.Fatalf("surrounding content lost:\n%s", out)
	}
	if !strings.Contains(out, "- old fact\n") || !strings.Contains(out, "- new fact\n") {
		t.Fatalf("facts not merged correctly:\n%s", out)
	}
}

func TestLLMIdentityCurator_Curate_PersistsGeneralSkillAndUserFacts(t *testing.T) {
	workspace := t.TempDir()
	provider := &testIdentityProvider{
		response: &providers.LLMResponse{
			Content: `{"soul":["Prefer native-name lookup first."],"user":["Prefers terse answers."]}`,
		},
	}
	curator := identityCuratorFor(t, provider, workspace)

	result, err := curator.Curate(context.Background(), IdentityCurateInput{
		Workspace:    workspace,
		TurnID:       "turn-1",
		Success:      true,
		UserMessage:  "What's the weather?",
		FinalContent: "Rain tomorrow.",
	})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if !result.Updated {
		t.Fatal("expected Updated=true")
	}
	if len(result.SoulFacts) != 1 || len(result.UserFacts) != 1 {
		t.Fatalf("result = %#v", result)
	}

	skill := string(mustReadFile(t, skills.SelfImprovementSkillPath(workspace)))
	if !strings.Contains(skill, skills.SelfImprovementWorkflow) || !strings.Contains(skill, "§ Prefer native-name lookup first.") {
		t.Fatalf("general skill not written:\n%s", skill)
	}
	if _, err := os.Stat(filepath.Join(workspace, "SOUL.md")); !os.IsNotExist(err) {
		t.Fatal("curation must not rewrite identity")
	}
	user := string(mustReadFile(t, filepath.Join(workspace, "USER.md")))
	if !strings.Contains(user, "§ Prefers terse answers.") {
		t.Fatalf("USER.md not written:\n%s", user)
	}
	if provider.chatCalls != 1 {
		t.Fatalf("expected 1 LLM call, got %d", provider.chatCalls)
	}
}

func TestLLMIdentityCurator_Curate_NoOpWithoutProvider(t *testing.T) {
	workspace := t.TempDir()
	curator := &LLMIdentityCurator{workspace: workspace, model: "m", cooldown: time.Minute, now: time.Now}
	result, err := curator.Curate(context.Background(), IdentityCurateInput{
		Workspace: workspace, Success: true, FinalContent: "hi",
	})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if result.Updated {
		t.Fatal("expected no-op without provider")
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "SOUL.md")); !os.IsNotExist(statErr) {
		t.Fatal("expected no SOUL.md written")
	}
}

func TestLLMIdentityCurator_Curate_NoopOnFailure(t *testing.T) {
	workspace := t.TempDir()
	provider := &testIdentityProvider{}
	curator := identityCuratorFor(t, provider, workspace)

	result, err := curator.Curate(context.Background(), IdentityCurateInput{
		Workspace: workspace, Success: false, FinalContent: "hi",
	})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if result.Updated {
		t.Fatal("expected no-op on failed turn")
	}
	if provider.chatCalls != 0 {
		t.Fatalf("expected 0 LLM calls on failure, got %d", provider.chatCalls)
	}
}

func TestLLMIdentityCurator_Curate_NoopOnCooldown(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	stampPath := filepath.Join(workspace, "memory", ".self-improvement-curated")
	mustWriteFile(t, stampPath, "curated", 0o600)

	provider := &testIdentityProvider{
		response: &providers.LLMResponse{Content: `{"soul":["x"],"user":[]}`},
	}
	curator := identityCuratorFor(t, provider, workspace)

	// A recent learning pass is within the five-minute cooldown.
	result, err := curator.Curate(context.Background(), IdentityCurateInput{
		Workspace: workspace, Success: true, FinalContent: "hi",
	})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if result.Updated {
		t.Fatal("expected cooldown no-op")
	}
	if provider.chatCalls != 0 {
		t.Fatalf("expected 0 LLM calls during cooldown, got %d", provider.chatCalls)
	}
}

func TestLLMIdentityCurator_Curate_NoopWhenLLMYieldsNothing(t *testing.T) {
	workspace := t.TempDir()
	provider := &testIdentityProvider{
		response: &providers.LLMResponse{Content: `{"soul":[],"user":[]}`},
	}
	curator := identityCuratorFor(t, provider, workspace)

	result, err := curator.Curate(context.Background(), IdentityCurateInput{
		Workspace: workspace, Success: true, FinalContent: "hi",
	})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if result.Updated {
		t.Fatal("expected no-op when LLM yields no facts")
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "SOUL.md")); !os.IsNotExist(statErr) {
		t.Fatal("expected no SOUL.md written")
	}
}

func TestLLMIdentityCurator_UsesSharedValidationAndBudget(t *testing.T) {
	workspace := t.TempDir()
	store := memory.NewCuratedStore(workspace)
	if err := store.AddEntry(memory.TargetUser, strings.Repeat("x", memory.UserBudget-5)); err != nil {
		t.Fatal(err)
	}
	provider := &testIdentityProvider{response: &providers.LLMResponse{
		Content: `{"learning":["password: should-not-persist","When debugging, reproduce the issue and verify the fix."],"user":["Prefers short responses."]}`,
	}}
	curator := identityCuratorFor(t, provider, workspace)
	result, err := curator.Curate(context.Background(), IdentityCurateInput{Workspace: workspace, Success: true, FinalContent: "Checked."})
	if err == nil {
		t.Fatal("expected secret/capacity rejection to be surfaced")
	}
	if !result.Updated || len(result.SoulFacts) != 1 || len(result.UserFacts) != 0 {
		t.Fatalf("result must only report actual writes: %#v", result)
	}
	if got := store.ReadEntries(memory.TargetLearning); len(got) != 1 || strings.Contains(got[0], "password") {
		t.Fatalf("invalid rules stored: %v", got)
	}
	if got := store.ReadEntries(memory.TargetUser); len(got) != 1 {
		t.Fatal("curator bypassed user budget")
	}
}

func TestLLMIdentityCurator_Curate_NoopOnEmptyFinalContent(t *testing.T) {
	workspace := t.TempDir()
	provider := &testIdentityProvider{}
	curator := identityCuratorFor(t, provider, workspace)

	if _, err := curator.Curate(context.Background(), IdentityCurateInput{
		Workspace: workspace, Success: true, FinalContent: "",
	}); err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if provider.chatCalls != 0 {
		t.Fatalf("expected 0 LLM calls for empty content, got %d", provider.chatCalls)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func mustWriteFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
