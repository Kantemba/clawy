package tools

import (
	"context"
	"fmt"
	"strings"
)

// Memory targets exposed to the LLM. Values match agent.MemoryTarget* so both
// layers share one vocabulary; they are duplicated here because pkg/tools
// cannot import pkg/agent (import cycle).
const (
	MemoryToolTargetAgent = "memory"
	MemoryToolTargetUser  = "user"
)

// MemoryBackend is the curated-memory contract the memory tool operates on.
// It is implemented by *agent.MemoryStore.
type MemoryBackend interface {
	AddEntry(target, text string) error
	ReplaceEntry(target, oldText, newText string) error
	RemoveEntry(target, oldText string) error
	ReadEntries(target string) []string
}

// MemoryTool is the EXCLUSIVE interface to the agent's persistent memory,
// modeled after Hermes' built-in memory tool: a small set of explicit actions
// (add / replace / remove / read) over two bounded targets — the agent's
// private notes and the user profile. Memory contents are preloaded into the
// system prompt every session, so anything stored here makes future sessions
// better informed: this is the core "improve as we interact" loop.
type MemoryTool struct {
	backend MemoryBackend
}

// NewMemoryTool builds a memory tool on top of the given backend.
func NewMemoryTool(backend MemoryBackend) *MemoryTool {
	return &MemoryTool{backend: backend}
}

func (t *MemoryTool) Name() string {
	return "memory"
}

func (t *MemoryTool) Description() string {
	return "Your persistent memory across sessions (preloaded into every conversation). Two targets: " +
		"'memory' = your private working notes (lessons learned, project state, decisions, gotchas); " +
		"'user' = stable facts about the user (name, role, preferences, environment, goals). " +
		"Actions: add (store a NEW short single-fact entry), replace (edit an existing entry via old_text), " +
		"remove (delete an entry via old_text), read (dump a target's current entries). " +
		"Entries are capped (~2.2k chars for 'memory', ~1.4k for 'user') so keep them terse and consolidated — " +
		"when full you MUST remove or replace stale entries before adding new ones. " +
		"NEVER store credentials, API keys, or secrets. " +
		"Use proactively: whenever you learn something durable about the user or the task, persist it."
}

func (t *MemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"add", "replace", "remove", "read"},
				"description": "Operation to perform on the memory target",
			},
			"target": map[string]any{
				"type":        "string",
				"enum":        []string{MemoryToolTargetAgent, MemoryToolTargetUser},
				"description": "'memory' = your private notes (MEMORY.md); 'user' = user profile (USER.md)",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "For action=add: the new entry text (one short fact per entry)",
			},
			"old_text": map[string]any{
				"type":        "string",
				"description": "For action=replace/remove: text matching (a substring of) the existing entry to update/delete",
			},
			"new_text": map[string]any{
				"type":        "string",
				"description": "For action=replace: the replacement text (empty deletes the matched entry)",
			},
		},
		"required": []string{"action", "target"},
	}
}

func (t *MemoryTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.backend == nil {
		return ErrorResult("memory system is unavailable")
	}

	action, _ := args["action"].(string)
	target, _ := args["target"].(string)
	action = strings.TrimSpace(action)
	target = strings.TrimSpace(target)

	if target != MemoryToolTargetAgent && target != MemoryToolTargetUser {
		return ErrorResult(fmt.Sprintf("unknown target %q: use 'memory' or 'user'", target))
	}

	switch action {
	case "read":
		entries := t.backend.ReadEntries(target)
		if len(entries) == 0 {
			return SilentResult(fmt.Sprintf("Memory target %q is empty.", target))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Contents of %s:\n", target)
		for _, e := range entries {
			sb.WriteString(entryMarker + " ")
			sb.WriteString(e)
			sb.WriteString("\n")
		}
		return SilentResult(sb.String())

	case "add":
		text, _ := args["text"].(string)
		if strings.TrimSpace(text) == "" {
			return ErrorResult("text is required for action=add")
		}
		if err := t.backend.AddEntry(target, text); err != nil {
			return ErrorResult(err.Error())
		}
		return SilentResult(fmt.Sprintf("Added to %q: %s", target, strings.TrimSpace(text)))

	case "replace":
		oldText, _ := args["old_text"].(string)
		newText, _ := args["new_text"].(string)
		if strings.TrimSpace(oldText) == "" {
			return ErrorResult("old_text is required for action=replace")
		}
		if err := t.backend.ReplaceEntry(target, oldText, newText); err != nil {
			return ErrorResult(err.Error())
		}
		if strings.TrimSpace(newText) == "" {
			return SilentResult(fmt.Sprintf("Removed from %q (matched %q).", target, strings.TrimSpace(oldText)))
		}
		return SilentResult(fmt.Sprintf("Updated in %q.", target))

	case "remove":
		oldText, _ := args["old_text"].(string)
		if strings.TrimSpace(oldText) == "" {
			return ErrorResult("old_text is required for action=remove")
		}
		if err := t.backend.RemoveEntry(target, oldText); err != nil {
			return ErrorResult(err.Error())
		}
		return SilentResult(fmt.Sprintf("Removed from %q (matched %q).", target, strings.TrimSpace(oldText)))

	default:
		return ErrorResult(fmt.Sprintf("unknown action %q: use add, replace, remove, or read", action))
	}
}

// entryMarker matches the on-disk entry prefix used by the memory store.
const entryMarker = "§"
