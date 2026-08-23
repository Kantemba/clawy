package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/Kantemba/clawy/pkg/providers"
)

// fakeSessionStore is a minimal in-memory SessionStore for tool tests.
type fakeSessionStore struct {
	sessions map[string][]providers.Message
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{sessions: make(map[string][]providers.Message)}
}

func (f *fakeSessionStore) AddMessage(key, role, content string) {
	f.sessions[key] = append(f.sessions[key], providers.Message{Role: role, Content: content})
}
func (f *fakeSessionStore) AddFullMessage(key string, msg providers.Message) {
	f.sessions[key] = append(f.sessions[key], msg)
}
func (f *fakeSessionStore) GetHistory(key string) []providers.Message { return f.sessions[key] }
func (f *fakeSessionStore) GetSummary(string) string                 { return "" }
func (f *fakeSessionStore) SetSummary(string, string)                {}
func (f *fakeSessionStore) SetHistory(key string, history []providers.Message) {
	f.sessions[key] = history
}
func (f *fakeSessionStore) TruncateHistory(key string, keepLast int) {
	msgs := f.sessions[key]
	if keepLast <= 0 {
		f.sessions[key] = nil
		return
	}
	if keepLast < len(msgs) {
		f.sessions[key] = msgs[len(msgs)-keepLast:]
	}
}
func (f *fakeSessionStore) Save(string) error { return nil }
func (f *fakeSessionStore) ListSessions() []string {
	keys := make([]string, 0, len(f.sessions))
	for k := range f.sessions {
		keys = append(keys, k)
	}
	return keys
}
func (f *fakeSessionStore) Close() error { return nil }

func seedSession(t *testing.T, store *fakeSessionStore, key string, msgs [][2]string) {
	t.Helper()
	for _, m := range msgs {
		store.AddMessage(key, m[0], m[1])
	}
}

func TestSessionSearchRanksAcrossSessions(t *testing.T) {
	store := newFakeSessionStore()
	seedSession(t, store, "wa:alice", [][2]string{
		{"user", "What is the deployment command for the billing service?"},
		{"assistant", "Use: kubectl rollout restart deploy/billing -n prod"},
	})
	seedSession(t, store, "wa:bob", [][2]string{
		{"user", "Tell me about cats"},
		{"assistant", "Cats are domesticated felines."},
	})

	tool := NewSessionSearchTool(store)
	result := tool.Execute(context.Background(), map[string]any{
		"query": "billing deployment kubectl",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "kubectl rollout restart deploy/billing") {
		t.Fatalf("expected billing hit in results, got:\n%s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "wa:alice") {
		t.Fatalf("expected session key in results, got:\n%s", result.ForLLM)
	}
	if strings.Contains(result.ForLLM, "felines") {
		t.Fatalf("unrelated session should not rank for billing query, got:\n%s", result.ForLLM)
	}
}

func TestSessionSearchNoDataAndNoMatch(t *testing.T) {
	store := newFakeSessionStore()
	tool := NewSessionSearchTool(store)

	result := tool.Execute(context.Background(), map[string]any{"query": "anything"})
	if result.IsError || !strings.Contains(result.ForLLM, "No past conversation data") {
		t.Fatalf("expected empty-store notice, got: %s", result.ForLLM)
	}

	seedSession(t, store, "s1", [][2]string{{"user", "hello world"}})
	result = tool.Execute(context.Background(), map[string]any{"query": "quantum entanglement"})
	if result.IsError || !strings.Contains(result.ForLLM, "No matches") {
		t.Fatalf("expected no-match notice, got: %s", result.ForLLM)
	}
}

func TestSessionSearchRequiresQuery(t *testing.T) {
	tool := NewSessionSearchTool(newFakeSessionStore())
	result := tool.Execute(context.Background(), map[string]any{"query": "   "})
	if !result.IsError || !strings.Contains(result.ForLLM, "query is required") {
		t.Fatalf("expected query validation error, got: %s", result.ForLLM)
	}
}

func TestSessionSearchSnippetTruncation(t *testing.T) {
	long := strings.Repeat("billing ", 200) // 1600 chars
	store := newFakeSessionStore()
	seedSession(t, store, "s1", [][2]string{{"user", long}})

	tool := NewSessionSearchTool(store)
	result := tool.Execute(context.Background(), map[string]any{"query": "billing"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	// The snippet must stay bounded even though the source message is huge.
	if len(result.ForLLM) > 4000 {
		t.Fatalf("result not truncated: %d chars", len(result.ForLLM))
	}
	if !strings.Contains(result.ForLLM, "…") {
		t.Fatalf("expected ellipsis in truncated snippet, got:\n%s", result.ForLLM)
	}
}
