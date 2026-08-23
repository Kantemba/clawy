package evolution_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/evolution"
	"github.com/Kantemba/clawy/pkg/skills"
)

const sampleSkillBody = `---
name: deploy-site
description: Deploy a static site to the staging server.
---

# deploy-site

## Procedure

1. Run ` + "`make build`" + ` in the repo root.
2. Sync artifacts to staging via ` + "`rsync -av dist/ user@stg:/var/www`" + `.
3. Run ` + "`make postdeploy`" + ` on the target host.
`

func writeSampleSkill(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "skills", "deploy-site")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "SKILL.md")
	require.NoError(t, os.WriteFile(path, []byte(sampleSkillBody), 0o644))
	return path
}

// stubRevisor injects a deterministic revision without an LLM.
type stubRevisor struct {
	draft evolution.SkillDraft
	err   error
	calls int
}

func (s *stubRevisor) Revise(_ context.Context, _ evolution.SkillFeedbackInput, _ []skills.SkillInfo) (evolution.SkillDraft, error) {
	s.calls++
	return s.draft, s.err
}

func newTestRuntime(t *testing.T, cfg config.EvolutionConfig, opts ...func(*evolution.RuntimeOptions)) *evolution.Runtime {
	t.Helper()
	o := evolution.RuntimeOptions{Config: cfg}
	for _, opt := range opts {
		opt(&o)
	}
	rt, err := evolution.NewRuntime(o)
	require.NoError(t, err)
	return rt
}

func baseTurnInput(workspace, status string, activeSkills []string, toolExecs []evolution.ToolExecutionRecord) evolution.TurnCaseInput {
	return evolution.TurnCaseInput{
		Workspace:        workspace,
		WorkspaceID:      workspace,
		TurnID:           "t-001",
		SessionKey:       "sess-1",
		AgentID:          "agent-1",
		Status:           status,
		UserMessage:      "Deploy the site to staging",
		FinalContent:     "deployed",
		ToolKinds:        []string{"bash", "fs"},
		ToolExecutions:   toolExecs,
		ActiveSkillNames: activeSkills,
	}
}

func withRevisor(r evolution.SkillRevisor) func(*evolution.RuntimeOptions) {
	return func(o *evolution.RuntimeOptions) { o.Revisor = r }
}

func withApplier(workspace string) func(*evolution.RuntimeOptions) {
	return func(o *evolution.RuntimeOptions) {
		o.Applier = evolution.NewApplier(evolution.NewPaths(workspace, ""), nil)
	}
}

func observeCfg() config.EvolutionConfig {
	return config.EvolutionConfig{Enabled: true, Mode: "observe", OnlineRevision: true}
}

func applyCfg() config.EvolutionConfig {
	return config.EvolutionConfig{Enabled: true, Mode: "apply", OnlineRevision: true}
}

func TestReviseSkillOnFailure_DisabledReturnsZero(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, config.EvolutionConfig{Enabled: false}, withRevisor(&stubRevisor{}))
	result, err := rt.ReviseSkillOnFailure(context.Background(), baseTurnInput(root, "error",
		[]string{"deploy-site"}, nil))
	require.NoError(t, err)
	assert.False(t, result.Applied)
	assert.Empty(t, result.Draft.ID)
}

func TestReviseSkillOnFailure_OnlineRevisionOffReturnsZero(t *testing.T) {
	root := t.TempDir()
	cfg := config.EvolutionConfig{Enabled: true, Mode: "observe", OnlineRevision: false}
	rt := newTestRuntime(t, cfg, withRevisor(&stubRevisor{draft: evolution.SkillDraft{TargetSkillName: "x"}}))
	result, err := rt.ReviseSkillOnFailure(context.Background(), baseTurnInput(root, "error",
		[]string{"deploy-site"}, nil))
	require.NoError(t, err)
	assert.False(t, result.Applied)
	assert.Empty(t, result.Draft.ID)
}

func TestReviseSkillOnFailure_SuccessTurnIsNoOp(t *testing.T) {
	root := t.TempDir()
	revisor := &stubRevisor{draft: evolution.SkillDraft{TargetSkillName: "deploy-site"}}
	rt := newTestRuntime(t, observeCfg(), withRevisor(revisor))
	result, err := rt.ReviseSkillOnFailure(context.Background(), baseTurnInput(root, "completed",
		[]string{"deploy-site"}, nil))
	require.NoError(t, err)
	assert.False(t, result.Applied)
	assert.Empty(t, result.Draft.ID)
	assert.Zero(t, revisor.calls)
}

func TestReviseSkillOnFailure_NoActiveSkillsIsNoOp(t *testing.T) {
	root := t.TempDir()
	revisor := &stubRevisor{draft: evolution.SkillDraft{TargetSkillName: "x"}}
	rt := newTestRuntime(t, observeCfg(), withRevisor(revisor))
	result, err := rt.ReviseSkillOnFailure(context.Background(), baseTurnInput(root, "error", nil, nil))
	require.NoError(t, err)
	assert.False(t, result.Applied)
	assert.Empty(t, result.Draft.ID)
	assert.Zero(t, revisor.calls)
}

func TestReviseSkillOnFailure_DraftSavedAsCandidateInObserveMode(t *testing.T) {
	root := t.TempDir()
	writeSampleSkill(t, root)
	revisor := &stubRevisor{draft: evolution.SkillDraft{
		TargetSkillName: "deploy-site",
		DraftType:       evolution.DraftTypeShortcut,
		ChangeKind:      evolution.ChangeKindMerge,
		HumanSummary:    "handle rsync permission errors",
		BodyOrPatch:     "## Revision Notes\n\nRetry with sudo on the target.",
	}}
	rt := newTestRuntime(t, observeCfg(), withRevisor(revisor), withApplier(root))

	input := baseTurnInput(root, "error", []string{"deploy-site"}, []evolution.ToolExecutionRecord{
		{Name: "bash", Success: false, ErrorSummary: "permission denied on rsync"},
	})
	result, err := rt.ReviseSkillOnFailure(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, revisor.calls)
	assert.Equal(t, "deploy-site", result.Draft.TargetSkillName)
	assert.Equal(t, evolution.ChangeKindMerge, result.Draft.ChangeKind)
	assert.False(t, result.Applied)

	// observe mode must not write the patch to disk.
	recaller := evolution.NewSkillsRecaller(root)
	unchanged, ok := recaller.LoadSkill("deploy-site")
	require.True(t, ok)
	assert.False(t, strings.Contains(unchanged, "Revision Notes"))
}

func TestReviseSkillOnFailure_AppliesInApplyModeAndWritesSkill(t *testing.T) {
	root := t.TempDir()
	writeSampleSkill(t, root)
	revisor := &stubRevisor{draft: evolution.SkillDraft{
		TargetSkillName: "deploy-site",
		DraftType:       evolution.DraftTypeShortcut,
		ChangeKind:      evolution.ChangeKindMerge,
		HumanSummary:    "handle rsync permission errors",
		BodyOrPatch:     "## Revision Notes\n\nRetry with sudo on the target.",
	}}
	rt := newTestRuntime(t, applyCfg(), withRevisor(revisor), withApplier(root))

	input := baseTurnInput(root, "error", []string{"deploy-site"}, nil)
	result, err := rt.ReviseSkillOnFailure(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, revisor.calls)
	assert.True(t, result.Applied, "apply mode should write the revision to disk")
	assert.NotEmpty(t, result.Draft.SourceRecordID)

	// The skill body on disk must now contain the merged revision section.
	recaller := evolution.NewSkillsRecaller(root)
	updated, ok := recaller.LoadSkill("deploy-site")
	require.True(t, ok)
	assert.Contains(t, updated, "## Revision Notes")
}

func TestReviseSkillOnFailure_InvalidDraftFromRevisorIsNoOp(t *testing.T) {
	root := t.TempDir()
	writeSampleSkill(t, root)
	// Draft missing required fields (no human_summary / body_or_patch) -> ValidateDraft fails.
	revisor := &stubRevisor{draft: evolution.SkillDraft{TargetSkillName: "deploy-site"}}
	rt := newTestRuntime(t, applyCfg(), withRevisor(revisor), withApplier(root))
	input := baseTurnInput(root, "error", []string{"deploy-site"}, nil)
	result, err := rt.ReviseSkillOnFailure(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, revisor.calls)
	assert.False(t, result.Applied)
	assert.Empty(t, result.Draft.ID)

	// Disk must be unchanged.
	recaller := evolution.NewSkillsRecaller(root)
	unchanged, ok := recaller.LoadSkill("deploy-site")
	require.True(t, ok)
	assert.False(t, strings.Contains(unchanged, "Revision Notes"))
}

func TestReviseSkillOnFailure_SkipsSkillWithNoBody(t *testing.T) {
	root := t.TempDir()
	revisor := &stubRevisor{draft: evolution.SkillDraft{TargetSkillName: "ghost"}}
	rt := newTestRuntime(t, applyCfg(), withRevisor(revisor), withApplier(root))
	input := baseTurnInput(root, "error", []string{"ghost"}, nil)
	result, err := rt.ReviseSkillOnFailure(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, result.Applied)
	assert.Empty(t, result.Draft.ID)
}

func TestReviseSkillOnFailure_DropsInvalidSkillNames(t *testing.T) {
	root := t.TempDir()
	writeSampleSkill(t, root)
	revisor := &stubRevisor{draft: evolution.SkillDraft{
		TargetSkillName: "deploy-site",
		DraftType:       evolution.DraftTypeShortcut,
		ChangeKind:      evolution.ChangeKindMerge,
		HumanSummary:    "handle rsync permission errors",
		BodyOrPatch:     "## Revision Notes\n\nRetry with sudo on the target.",
	}}
	rt := newTestRuntime(t, applyCfg(), withRevisor(revisor), withApplier(root))
	// Leading invalid names should be skipped; the valid "deploy-site" with a body
	// is still reachable and gets revised.
	input := baseTurnInput(root, "error", []string{"../escape", "bad name", "deploy-site"}, nil)
	result, err := rt.ReviseSkillOnFailure(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, revisor.calls)
	assert.Equal(t, "deploy-site", result.Draft.TargetSkillName)
}

func TestLLMSkillRevisor_FallsBackToHeuristic(t *testing.T) {
	feedback := evolution.SkillFeedbackInput{
		SkillName:      "deploy-site",
		SkillBody:      sampleSkillBody,
		TaskSummary:    "Deploy the site",
		FailureSummary: "bash: permission denied",
		FinalOutput:    "rsync failed",
		ToolExecutions: []evolution.ToolExecutionRecord{
			{Name: "bash", Success: false, ErrorSummary: "permission denied on rsync"},
		},
	}
	// No provider -> falls back to HeuristicSkillRevisor.
	r := evolution.NewLLMSkillRevisor(nil, "gpt-4o-mini", evolution.HeuristicSkillRevisor{})
	draft, err := r.Revise(context.Background(), feedback, nil)
	require.NoError(t, err)
	assert.Equal(t, "deploy-site", draft.TargetSkillName)
	assert.Equal(t, evolution.ChangeKindMerge, draft.ChangeKind)
	assert.Contains(t, draft.BodyOrPatch, "Revision Notes")
	assert.Contains(t, draft.BodyOrPatch, "permission denied")
}

func TestLLMSkillRevisor_NilReceiverReturnsZero(t *testing.T) {
	var r *evolution.LLMSkillRevisor
	draft, err := r.Revise(context.Background(), evolution.SkillFeedbackInput{SkillName: "x"}, nil)
	require.NoError(t, err)
	assert.Empty(t, draft.TargetSkillName)
}

func TestHeuristicSkillRevisor_EmptyFailureSummaryIsNoOp(t *testing.T) {
	h := evolution.HeuristicSkillRevisor{}
	draft, err := h.Revise(context.Background(), evolution.SkillFeedbackInput{
		SkillName: "deploy-site",
		SkillBody: sampleSkillBody,
	}, nil)
	require.NoError(t, err)
	assert.Empty(t, draft.TargetSkillName)
}

func TestHeuristicSkillRevisor_RendersToolExecutions(t *testing.T) {
	h := evolution.HeuristicSkillRevisor{}
	draft, err := h.Revise(context.Background(), evolution.SkillFeedbackInput{
		SkillName:      "deploy-site",
		SkillBody:      sampleSkillBody,
		FailureSummary: "bash: permission denied",
		FinalOutput:    "rsync failed",
		ToolExecutions: []evolution.ToolExecutionRecord{
			{Name: "bash", Success: false, ErrorSummary: "permission denied on rsync"},
			{Name: "fs", Success: true},
		},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "deploy-site", draft.TargetSkillName)
	assert.Contains(t, draft.BodyOrPatch, "permission denied on rsync")
	assert.Contains(t, draft.BodyOrPatch, "fs")
}

// stubCurator records calls and returns a canned result, mirroring stubRevisor.
type stubCurator struct {
	result evolution.IdentityCurateResult
	err    error
	calls  int
}

func (c *stubCurator) Curate(_ context.Context, _ evolution.IdentityCurateInput) (evolution.IdentityCurateResult, error) {
	c.calls++
	return c.result, c.err
}

func withCurator(c evolution.IdentityCurator) func(*evolution.RuntimeOptions) {
	return func(o *evolution.RuntimeOptions) { o.Curator = c }
}

func TestCurateIdentity_DisabledReturnsZero(t *testing.T) {
	root := t.TempDir()
	curator := &stubCurator{}
	rt := newTestRuntime(t, config.EvolutionConfig{Enabled: false, IdentityCuration: true}, withCurator(curator))
	result, err := rt.CurateIdentity(context.Background(), evolution.IdentityCurateInput{
		Workspace: root, Success: true, FinalContent: "resolved the issue",
	})
	require.NoError(t, err)
	assert.False(t, result.Updated)
	assert.Zero(t, curator.calls)
}

func TestCurateIdentity_CurationOffReturnsZero(t *testing.T) {
	root := t.TempDir()
	curator := &stubCurator{}
	rt := newTestRuntime(t, config.EvolutionConfig{Enabled: true, IdentityCuration: false}, withCurator(curator))
	result, err := rt.CurateIdentity(context.Background(), evolution.IdentityCurateInput{
		Workspace: root, Success: true, FinalContent: "resolved the issue",
	})
	require.NoError(t, err)
	assert.False(t, result.Updated)
	assert.Zero(t, curator.calls)
}

func TestCurateIdentity_NoopOnFailure(t *testing.T) {
	root := t.TempDir()
	curator := &stubCurator{}
	rt := newTestRuntime(t, config.EvolutionConfig{Enabled: true, IdentityCuration: true}, withCurator(curator))
	result, err := rt.CurateIdentity(context.Background(), evolution.IdentityCurateInput{
		Workspace: root, Success: false, FinalContent: "resolved the issue",
	})
	require.NoError(t, err)
	assert.False(t, result.Updated)
	assert.Zero(t, curator.calls, "curator must not run for unsuccessful turns")
}

func TestCurateIdentity_DelegatesToCuratorOnSuccess(t *testing.T) {
	root := t.TempDir()
	curator := &stubCurator{result: evolution.IdentityCurateResult{Updated: true, SoulFacts: []string{"fact"}}}
	rt := newTestRuntime(t, config.EvolutionConfig{Enabled: true, IdentityCuration: true}, withCurator(curator))
	result, err := rt.CurateIdentity(context.Background(), evolution.IdentityCurateInput{
		Workspace: root, TurnID: "t-1", Success: true, UserMessage: "hi", FinalContent: "done",
	})
	require.NoError(t, err)
	assert.True(t, result.Updated)
	assert.Equal(t, []string{"fact"}, result.SoulFacts)
	assert.Equal(t, 1, curator.calls)
}

func TestCurateIdentity_NilCuratorIsZero(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, config.EvolutionConfig{Enabled: true, IdentityCuration: true})
	result, err := rt.CurateIdentity(context.Background(), evolution.IdentityCurateInput{
		Workspace: root, Success: true, FinalContent: "done",
	})
	require.NoError(t, err)
	assert.False(t, result.Updated)
}
