// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors
//
// Memory model modeled after Hermes (NousResearch): two small, curated,
// always-preloaded stores that the agent itself maintains through the
// exclusive `memory` tool. Bounded capacity forces the agent to consolidate
// and prioritize what it learns, which is what makes the agent improve with
// every interaction instead of accumulating unbounded notes.

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Kantemba/clawy/pkg/fileutil"
)

// Memory targets. The same vocabulary is exposed to the LLM via the `memory`
// tool, so these constants double as the tool-level target names.
const (
	// MemoryTargetAgent is the agent's private working memory (MEMORY.md):
	// lessons learned, project state, user-specific gotchas, reminders.
	MemoryTargetAgent = "memory"
	// MemoryTargetUser is the user profile (USER.md): stable facts about the
	// person the agent talks to — name, preferences, environment, goals.
	MemoryTargetUser = "user"
)

// Hermes-derived capacity budgets (in characters). Small budgets are the
// point: they force curation instead of hoarding.
const (
	// DefaultAgentMemoryBudget is the char budget for MEMORY.md.
	DefaultAgentMemoryBudget = 2200
	// DefaultUserMemoryBudget is the char budget for USER.md.
	DefaultUserMemoryBudget = 1375
)

// entryMarker marks the start of a memory entry. Entries render one per line,
// each prefixed with "§" — the same convention Hermes uses so the model can
// parse and manage its own memory reliably.
const entryMarker = "§"

// MemoryStore manages the agent's curated persistent memory:
//   - agent memory: memory/MEMORY.md
//   - user profile: USER.md
//
// Files are split into "§"-prefixed entries. Legacy free-form content (no §
// markers) is treated as a single blob entry so existing workspaces keep
// working; the agent can consolidate it via the memory tool at its own pace.
type MemoryStore struct {
	workspace string
	mu        sync.Mutex
}

// NewMemoryStore creates a MemoryStore rooted at the given workspace and
// ensures the memory directory exists.
func NewMemoryStore(workspace string) *MemoryStore {
	os.MkdirAll(filepath.Join(workspace, "memory"), 0o755)
	return &MemoryStore{workspace: workspace}
}

// Path returns the file backing a memory target. Unknown targets return "".
func (ms *MemoryStore) Path(target string) string {
	switch target {
	case MemoryTargetAgent:
		return filepath.Join(ms.workspace, "memory", "MEMORY.md")
	case MemoryTargetUser:
		return filepath.Join(ms.workspace, "USER.md")
	default:
		return ""
	}
}

// Budget returns the character capacity for a memory target (0 if unknown).
func (ms *MemoryStore) Budget(target string) int {
	switch target {
	case MemoryTargetAgent:
		return DefaultAgentMemoryBudget
	case MemoryTargetUser:
		return DefaultUserMemoryBudget
	default:
		return 0
	}
}

// readRaw returns the raw file content for a target, or "" when absent.
func (ms *MemoryStore) readRaw(target string) string {
	path := ms.Path(target)
	if path == "" {
		return ""
	}
	if data, err := os.ReadFile(path); err == nil {
		return string(data)
	}
	return ""
}

// ParseEntries splits raw memory file content into entries.
//
// Content with "§"-prefixed lines is split per entry. Content without any
// marker (legacy free-form markdown) is returned as a single entry so nothing
// is lost or silently reformatted.
func ParseEntries(raw string) []string {
	raw = strings.TrimLeft(raw, "\ufeff")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if !strings.Contains(raw, "\n"+entryMarker) && !strings.HasPrefix(raw, entryMarker) {
		// Legacy blob: keep as one entry.
		return []string{strings.TrimRight(raw, "\n")}
	}
	var entries []string
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, entryMarker) {
			entries = append(entries, strings.TrimPrefix(line, entryMarker+" "))
			continue
		}
		if len(entries) == 0 {
			// Preamble before the first entry (e.g. an old heading): keep it
			// attached as the head of the first entry.
			if strings.TrimSpace(line) != "" {
				entries = append(entries, line)
			}
			continue
		}
		entries[len(entries)-1] += "\n" + line
	}
	// Trim trailing whitespace per entry and drop empties.
	result := make([]string, 0, len(entries))
	for _, e := range entries {
		e = strings.TrimRight(e, " \t\n")
		if strings.TrimSpace(e) != "" {
			result = append(result, e)
		}
	}
	return result
}

// renderEntries joins entries into the on-disk / prompt format.
func renderEntries(entries []string) string {
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		e = strings.TrimRight(e, " \t\n")
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, entryMarker) {
			e = entryMarker + " " + e
		}
		lines = append(lines, e)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// ReadEntries returns the parsed entries for a target (nil if empty/unknown).
func (ms *MemoryStore) ReadEntries(target string) []string {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ParseEntries(ms.readRaw(target))
}

// writeRaw atomically persists rendered content for a target.
func (ms *MemoryStore) writeRaw(target, content string) error {
	path := ms.Path(target)
	if path == "" {
		return fmt.Errorf("unknown memory target %q", target)
	}
	// 0o600 matches the rest of the memory layer: owner read/write only.
	return fileutil.WriteFileAtomic(path, []byte(content), 0o600)
}

// normalizeForDuplicate collapses whitespace/case so trivial rewordings of an
// existing entry are caught.
func normalizeForDuplicate(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// secretPatterns match common credential shapes. Memory is preloaded into
// every future prompt, so it must never become a secrets store.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(api[_-]?key|apikey|secret|token|password|passwd|pwd)\b\s*[:=]\s*\S+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),             // OpenAI-style keys
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),        // GitHub tokens
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),                  // AWS access keys
	regexp.MustCompile(`\bBearer\s+[A-Za-z0-9._~+/-]{16,}=*\b`), // Bearer headers
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),    // PEM blocks
}

// validateEntryText rejects content that must not enter persistent memory.
func validateEntryText(text string) error {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return fmt.Errorf("memory entry text is empty")
	}
	for _, re := range secretPatterns {
		if match := re.FindString(trimmed); match != "" {
			return fmt.Errorf(
				"refusing to store: text looks like a credential or secret (matched %q). "+
					"Never persist secrets in memory; ask the user to keep them in a config or env file",
				match,
			)
		}
	}
	return nil
}

// capacityError explains the budget and how to resolve it — Hermes-style: the
// agent is expected to consolidate its own memory rather than the runtime
// silently truncating or auto-compacting it.
func capacityError(target string, budget, used, incoming int) error {
	return fmt.Errorf(
		"memory target %q is full: %d/%d chars used, new content needs %d more. "+
			"Consolidate first: use action=remove or action=replace to drop outdated or redundant entries, then retry",
		target, used, budget, incoming,
	)
}

// AddEntry appends a new entry to a target. Rejects duplicates, near-
// duplicates, secrets, and content that would exceed the target's capacity.
func (ms *MemoryStore) AddEntry(target, text string) error {
	if ms.Budget(target) == 0 {
		return fmt.Errorf("unknown memory target %q (use %q or %q)", target, MemoryTargetAgent, MemoryTargetUser)
	}
	if err := validateEntryText(text); err != nil {
		return err
	}
	text = strings.TrimSpace(text)

	ms.mu.Lock()
	defer ms.mu.Unlock()

	raw := ms.readRaw(target)
	entries := ParseEntries(raw)
	newNorm := normalizeForDuplicate(text)
	for _, e := range entries {
		eNorm := normalizeForDuplicate(e)
		switch {
		case eNorm == newNorm:
			return fmt.Errorf("entry already stored in %q (exact duplicate); nothing to do", target)
		case strings.Contains(eNorm, newNorm) || strings.Contains(newNorm, eNorm):
			return fmt.Errorf(
				"a similar entry already exists in %q; use action=replace with old_text to refine it instead of adding a near-duplicate",
				target,
			)
		}
	}

	budget := ms.Budget(target)
	rendered := renderEntries(append(append([]string(nil), entries...), text))
	if used, want := len([]rune(raw)), len([]rune(rendered)); want > budget {
		return capacityError(target, budget, used, want-used)
	}

	return ms.writeRaw(target, rendered)
}

// ReplaceEntry rewrites the first entry containing oldText. An empty newText
// removes the matched entry.
func (ms *MemoryStore) ReplaceEntry(target, oldText, newText string) error {
	if ms.Budget(target) == 0 {
		return fmt.Errorf("unknown memory target %q (use %q or %q)", target, MemoryTargetAgent, MemoryTargetUser)
	}
	if strings.TrimSpace(oldText) == "" {
		return fmt.Errorf("old_text is required")
	}
	newText = strings.TrimSpace(newText)
	if newText != "" {
		if err := validateEntryText(newText); err != nil {
			return err
		}
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	raw := ms.readRaw(target)
	entries := ParseEntries(raw)
	oldNorm := normalizeForDuplicate(oldText)
	matchIdx := -1
	for i, e := range entries {
		if strings.Contains(e, oldText) || strings.Contains(normalizeForDuplicate(e), oldNorm) {
			matchIdx = i
			break
		}
	}
	if matchIdx == -1 {
		return fmt.Errorf("old_text not found in %q; use action=read to inspect current entries first", target)
	}

	updated := append([]string(nil), entries...)
	if newText == "" {
		updated = append(updated[:matchIdx], updated[matchIdx+1:]...)
	} else if strings.Contains(entries[matchIdx], oldText) {
		updated[matchIdx] = strings.Replace(entries[matchIdx], oldText, newText, 1)
	} else {
		// Matched only after normalization (whitespace differs); swap the entry.
		updated[matchIdx] = newText
	}

	budget := ms.Budget(target)
	rendered := renderEntries(updated)
	if len([]rune(rendered)) > budget {
		return capacityError(target, budget, len([]rune(raw)), len([]rune(rendered))-len([]rune(raw)))
	}
	return ms.writeRaw(target, rendered)
}

// RemoveEntry deletes the first entry containing oldText.
func (ms *MemoryStore) RemoveEntry(target, oldText string) error {
	return ms.ReplaceEntry(target, oldText, "")
}

// GetMemoryContext renders the Hermes-style memory snapshot embedded in the
// system prompt. Both sections are always rendered (even when empty) so the
// model knows the memory system exists from the very first conversation —
// this priming is what starts the improve-with-use feedback loop.
func (ms *MemoryStore) GetMemoryContext() string {
	var sb strings.Builder

	sb.WriteString("# Memory\n\n")
	sb.WriteString("You have persistent memory that is preloaded into every conversation. ")
	sb.WriteString("Manage it ONLY with the `memory` tool; recall past conversations with `session_search`.\n\n")

	sb.WriteString("## MEMORY.md — your private notes\n")
	sb.WriteString("Durable lessons, project state, decisions, gotchas. Keep entries short, factual, and deduplicated. When full, consolidate: remove or replace stale entries before adding new ones. Never store credentials.\n")
	ms.writeSection(&sb, MemoryTargetAgent)

	sb.WriteString("\n## USER.md — what you know about the user\n")
	sb.WriteString("Stable facts about the user worth carrying across sessions: name, role, preferences, environment, ongoing goals. Update as you learn more; prune what goes stale.\n")
	ms.writeSection(&sb, MemoryTargetUser)

	return sb.String()
}

// writeSection renders one target's usage line + entries into the snapshot.
func (ms *MemoryStore) writeSection(sb *strings.Builder, target string) {
	raw := ms.readRaw(target)
	entries := ParseEntries(raw)

	switch {
	case len(entries) == 0:
		fmt.Fprintf(sb, "Current contents: (empty — record durable learnings here with the `memory` tool)\n\n")
	default:
		fmt.Fprintf(sb, "Current contents (%d/%d chars):\n", len([]rune(raw)), ms.Budget(target))
		for _, e := range entries {
			if !strings.HasPrefix(e, entryMarker) {
				sb.WriteString(entryMarker + " ")
			}
			sb.WriteString(e)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
}


