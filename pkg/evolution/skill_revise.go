package evolution

import (
	"context"
	"fmt"
	"strings"

	"github.com/Kantemba/clawy/pkg/providers"
	"github.com/Kantemba/clawy/pkg/skills"
)

// SkillRevisor produces a targeted revision of an existing skill from turn
// feedback. Unlike DraftGenerator (which creates brand-new skills from
// clustered patterns during the cold path), a SkillRevisor patches a skill
// that already exists on disk, so it can close the learning loop online — as
// soon as a skill-guided turn fails — rather than waiting for a batch cycle.
//
// Revisions are deliberately conservative: the revisor is asked for a
// ChangeKindMerge draft (it appends a corrective section to the skill rather
// than overwriting it), and every draft is run through ReviewDraft +
// validateAppliedSkillBody before it is applied to disk.
type SkillRevisor interface {
	Revise(ctx context.Context, feedback SkillFeedbackInput, matches []skills.SkillInfo) (SkillDraft, error)
}

// LLMSkillRevisor uses an LLM to turn failure feedback into a targeted merge
// draft for an existing skill. If the LLM is unavailable it falls back to a
// stub revisor (if configured) or a deterministic heuristic that records the
// failure summary as revision notes.
type LLMSkillRevisor struct {
	provider providers.LLMProvider
	model    string
	fallback SkillRevisor
}

// NewLLMSkillRevisor returns a revisor backed by provider/model, with the
// given fallback used when the LLM cannot produce a valid draft.
func NewLLMSkillRevisor(provider providers.LLMProvider, model string, fallback SkillRevisor) *LLMSkillRevisor {
	return &LLMSkillRevisor{
		provider: provider,
		model:    strings.TrimSpace(model),
		fallback: fallback,
	}
}

// Revise returns a merge draft targeting feedback.SkillName, or a zero draft
// (and nil error) when no revision can be produced.
func (r *LLMSkillRevisor) Revise(ctx context.Context, feedback SkillFeedbackInput, matches []skills.SkillInfo) (SkillDraft, error) {
	if r == nil || feedback.SkillName == "" {
		return SkillDraft{}, nil
	}

	if r.provider != nil && r.model != "" {
		callCtx, cancel := withLLMCallTimeout(ctx, llmDraftGenerationTimeout)
		defer cancel()
		resp, err := r.provider.Chat(callCtx, []providers.Message{
			{
				Role:    "system",
				Content: "Return exactly one JSON object for a skill revision. Do not use markdown fences.",
			},
			{
				Role:    "user",
				Content: buildSkillRevisePrompt(feedback, matches),
			},
		}, nil, r.model, map[string]any{"temperature": 0.2})
		if err == nil && resp != nil && strings.TrimSpace(resp.Content) != "" {
			if draft, ok := parseLLMDraft(strings.TrimSpace(resp.Content)); ok {
				if findings := ValidateDraft(draft); len(findings) == 0 && draft.TargetSkillName == feedback.SkillName {
					draft.ChangeKind = ChangeKindMerge
					return draft, nil
				}
			}
		}
	}

	if r.fallback != nil {
		return r.fallback.Revise(ctx, feedback, matches)
	}
	return HeuristicSkillRevisor{}.Revise(ctx, feedback, matches)
}

// HeuristicSkillRevisor produces a conservative merge draft from failure
// metadata without an LLM. It is the default fallback so the loop is useful
// even when no model is configured for revision.
type HeuristicSkillRevisor struct{}

func (HeuristicSkillRevisor) Revise(_ context.Context, feedback SkillFeedbackInput, _ []skills.SkillInfo) (SkillDraft, error) {
	if feedback.SkillName == "" || strings.TrimSpace(feedback.FailureSummary) == "" {
		return SkillDraft{}, nil
	}
	patch := formatHeuristicRevision(feedback)
	return SkillDraft{
		TargetSkillName: feedback.SkillName,
		DraftType:       DraftTypeShortcut,
		ChangeKind:      ChangeKindMerge,
		HumanSummary:    fmt.Sprintf("Revise %s after failed turn: %s", feedback.SkillName, feedback.FailureSummary),
		BodyOrPatch:     patch,
	}, nil
}

func formatHeuristicRevision(feedback SkillFeedbackInput) string {
	var b strings.Builder
	b.WriteString("## Revision Notes\n\n")
	b.WriteString("This skill was invoked during a task that did not succeed. ")
	b.WriteString("Review the failure context before relying on this revision.\n\n")
	b.WriteString("**Failure summary:** ")
	b.WriteString(strings.TrimSpace(feedback.FailureSummary))
	b.WriteString("\n")
	if trimmed := strings.TrimSpace(feedback.FinalOutput); trimmed != "" {
		b.WriteString("\n**Final output at failure:**\n\n")
		b.WriteString(trimmed)
		b.WriteString("\n")
	}
	if len(feedback.ToolExecutions) > 0 {
		b.WriteString("\n**Observed tool executions:**\n\n")
		for _, exec := range feedback.ToolExecutions {
			status := "ok"
			if !exec.Success {
				status = "failed"
			}
			b.WriteString(fmt.Sprintf("- %s (%s)", exec.Name, status))
			if exec.ErrorSummary != "" {
				b.WriteString(": " + exec.ErrorSummary)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

const skillReviseSystemPrompt = `You revise an existing agent skill (SKILL.md) so it handles a case where a turn
that used the skill failed. You are given the skill's current body and the
feedback from the failed turn.

Produce exactly one JSON object. Do not use markdown fences. Required fields:
- target_skill_name: must equal "` + "`feedback.SkillName`" + `" (the existing skill being revised).
- draft_type: "shortcut".
- change_kind: "merge" (append a corrective section to the existing skill; never overwrite).
- human_summary: one short sentence describing the gap being closed.
- body_or_patch: a Markdown section (with a "## ..." heading) to merge into the
  skill. It must describe, step by step, the exact corrective procedure to
  avoid the failure. Strip any prose that would duplicate guidance already in
  the skill; only add what the skill is missing. Do not reproduce learning
  traces or "validated examples" commentary.

Optional array fields (only include if genuinely useful): intended_use_cases,
preferred_entry_path, avoid_patterns.

Existing skill body:
---BEGIN SKILL---
`

func buildSkillRevisePrompt(feedback SkillFeedbackInput, matches []skills.SkillInfo) string {
	var b strings.Builder
	b.WriteString(skillReviseSystemPrompt)
	b.WriteString(strings.TrimSpace(feedback.SkillBody))
	b.WriteString("\n---END SKILL---\n\n")

	b.WriteString("Failed task goal: ")
	b.WriteString(fallbackString(feedback.TaskSummary, "unspecified"))
	b.WriteString("\n\n")
	b.WriteString("What went wrong: ")
	b.WriteString(fallbackString(feedback.FailureSummary, "the turn did not reach a successful conclusion"))
	b.WriteString("\n\n")
	if trimmed := strings.TrimSpace(feedback.FinalOutput); trimmed != "" {
		b.WriteString("Final output at failure:\n")
		b.WriteString(trimmed)
		b.WriteString("\n\n")
	}
	if len(feedback.ToolExecutions) > 0 {
		excerpts := make([]string, 0, len(feedback.ToolExecutions))
		for _, exec := range feedback.ToolExecutions {
			entry := fmt.Sprintf("- %s success=%t", exec.Name, exec.Success)
			if exec.ErrorSummary != "" {
				entry += " error=" + exec.ErrorSummary
			}
			excerpts = append(excerpts, entry)
		}
		b.WriteString("Tool executions: ")
		b.WriteString(strings.Join(excerpts, "\n"))
		b.WriteString("\n\n")
	}
	b.WriteString("Related matched skills: ")
	b.WriteString(summarizeSkillMatches(matches))
	b.WriteString("\n")
	return b.String()
}
