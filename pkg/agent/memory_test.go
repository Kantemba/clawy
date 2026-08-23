package agent

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func newTestMemoryStore(t *testing.T) *MemoryStore {
	t.Helper()
	return NewMemoryStore(t.TempDir())
}

func TestParseEntriesLegacyBlobKeptAsSingleEntry(t *testing.T) {
	raw := "# Memory\n\nUser likes Go.\nPrefers terse answers."
	entries := ParseEntries(raw)
	if len(entries) != 1 {
		t.Fatalf("expected legacy content to parse as one entry, got %d: %#v", len(entries), entries)
	}
	if !strings.Contains(entries[0], "User likes Go.") {
		t.Fatalf("legacy entry lost content: %q", entries[0])
	}
}

func TestParseEntriesMarkerFormat(t *testing.T) {
	raw := "§ First entry\n§ Second entry\nmulti-line tail"
	entries := ParseEntries(raw)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %#v", len(entries), entries)
	}
	if entries[0] != "First entry" {
		t.Fatalf("unexpected first entry %q", entries[0])
	}
	if !strings.Contains(entries[1], "Second entry\nmulti-line tail") {
		t.Fatalf("multiline continuation lost: %q", entries[1])
	}
}

func TestParseEntriesEmpty(t *testing.T) {
	for _, raw := range []string{"", "   \n\t"} {
		if entries := ParseEntries(raw); len(entries) != 0 {
			t.Fatalf("expected no entries for %q, got %#v", raw, entries)
		}
	}
}

func TestAddEntryAndReadBack(t *testing.T) {
	ms := newTestMemoryStore(t)

	if err := ms.AddEntry(MemoryTargetAgent, "User prefers Go over Python."); err != nil {
		t.Fatalf("AddEntry failed: %v", err)
	}

	entries := ms.ReadEntries(MemoryTargetAgent)
	if len(entries) != 1 || !strings.Contains(entries[0], "User prefers Go") {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	// Persisted in marker format on disk.
	data, err := os.ReadFile(ms.Path(MemoryTargetAgent))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), entryMarker+" ") {
		t.Fatalf("expected §-prefixed on-disk format, got %q", string(data))
	}
}

func TestAddEntryRejectsDuplicates(t *testing.T) {
	ms := newTestMemoryStore(t)
	text := "Deployment target is Fly.io."

	if err := ms.AddEntry(MemoryTargetAgent, text); err != nil {
		t.Fatalf("first add failed: %v", err)
	}
	if err := ms.AddEntry(MemoryTargetAgent, text); err == nil {
		t.Fatal("expected exact duplicate to be rejected")
	}
	// Whitespace/case-insensitive near-duplicate.
	if err := ms.AddEntry(MemoryTargetAgent, "  deployment TARGET is fly.io.  "); err == nil {
		t.Fatal("expected normalized duplicate to be rejected")
	}
}

func TestAddEntryRejectsSecrets(t *testing.T) {
	ms := newTestMemoryStore(t)
	for _, secret := range []string{
		"api_key=sk-abc123defghi4567jklmn",
		"password: hunter2supersecret",
		"-----BEGIN RSA PRIVATE KEY-----",
	} {
		if err := ms.AddEntry(MemoryTargetAgent, secret); err == nil {
			t.Fatalf("expected secret %q to be rejected", secret)
		}
	}
	if entries := ms.ReadEntries(MemoryTargetAgent); len(entries) != 0 {
		t.Fatalf("secrets must not be persisted, got %#v", entries)
	}
}

func TestAddEntryCapacityForcesConsolidation(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())

	filler := strings.Repeat("x", DefaultUserMemoryBudget/2+10)
	if err := ms.AddEntry(MemoryTargetUser, filler); err != nil {
		t.Fatalf("initial fill failed: %v", err)
	}
	err := ms.AddEntry(MemoryTargetUser, strings.Repeat("y", DefaultUserMemoryBudget/2))
	if err == nil {
		t.Fatal("expected capacity error when exceeding budget")
	}
	if !strings.Contains(err.Error(), "Consolidate first") {
		t.Fatalf("capacity error should instruct consolidation, got %v", err)
	}
}

func TestReplaceAndRemoveEntry(t *testing.T) {
	ms := newTestMemoryStore(t)

	if err := ms.AddEntry(MemoryTargetUser, "User lives in Berlin."); err != nil {
		t.Fatal(err)
	}
	if err := ms.ReplaceEntry(MemoryTargetUser, "Berlin", "Tokyo"); err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	entries := ms.ReadEntries(MemoryTargetUser)
	if len(entries) != 1 || !strings.Contains(entries[0], "Tokyo") {
		t.Fatalf("replace did not apply: %#v", entries)
	}

	if err := ms.RemoveEntry(MemoryTargetUser, "Tokyo"); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if entries := ms.ReadEntries(MemoryTargetUser); len(entries) != 0 {
		t.Fatalf("remove did not apply: %#v", entries)
	}
}

func TestReplaceEntryMissingOldTextErrors(t *testing.T) {
	ms := newTestMemoryStore(t)
	if err := ms.ReplaceEntry(MemoryTargetAgent, "nonexistent", "new"); err == nil {
		t.Fatal("expected error replacing missing old_text")
	}
}

func TestUnknownTargetRejected(t *testing.T) {
	ms := newTestMemoryStore(t)
	if err := ms.AddEntry("daily", "note"); err == nil {
		t.Fatal("expected unknown target to be rejected")
	}
	if err := ms.ReplaceEntry("daily", "a", "b"); err == nil {
		t.Fatal("expected unknown target to be rejected")
	}
}

func TestGetMemoryContextAlwaysRendersBothSections(t *testing.T) {
	ms := newTestMemoryStore(t)

	snapshot := ms.GetMemoryContext()
	if !strings.Contains(snapshot, "# Memory") {
		t.Fatalf("snapshot must include heading, got:\n%s", snapshot)
	}
	if !strings.Contains(snapshot, "MEMORY.md") || !strings.Contains(snapshot, "USER.md") {
		t.Fatalf("snapshot must always surface both stores, got:\n%s", snapshot)
	}
	if !strings.Contains(snapshot, "`memory` tool") {
		t.Fatalf("snapshot must teach the memory tool contract, got:\n%s", snapshot)
	}

	if err := ms.AddEntry(MemoryTargetUser, "Name is Ada."); err != nil {
		t.Fatal(err)
	}
	snapshot = ms.GetMemoryContext()
	if !strings.Contains(snapshot, "§ Name is Ada.") {
		t.Fatalf("snapshot must render user entries, got:\n%s", snapshot)
	}
}

func TestGetMemoryContextShowsCapacityWhenNonEmpty(t *testing.T) {
	ms := newTestMemoryStore(t)
	if err := ms.AddEntry(MemoryTargetAgent, "Lesson: always run tests."); err != nil {
		t.Fatal(err)
	}
	snapshot := ms.GetMemoryContext()
	wantCapacity := "/" + strconv.Itoa(DefaultAgentMemoryBudget) + " chars"
	if !strings.Contains(snapshot, wantCapacity) {
		t.Fatalf("snapshot should show capacity usage %q, got:\n%s", wantCapacity, snapshot)
	}
}

