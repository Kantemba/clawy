package agent

import (
	"os"
	"strings"
	"testing"

	"github.com/Kantemba/clawy/pkg/skills"
)

func TestSelfImprovementAlwaysLoadedWithOptionalSkillsOff(t *testing.T) {
	builder := NewContextBuilder(t.TempDir())
	for _, req := range []PromptBuildRequest{
		{CurrentMessage: "Hello"},
		{CurrentMessage: "Hello", SuppressSkillContext: true},
		{CurrentMessage: "Hello", AllowedSkills: []string{"another-skill"}},
		{CurrentMessage: "Hello", SuppressToolUseRule: true},
		{CurrentMessage: "Hello", ActiveSkills: []string{skills.SelfImprovementSkillName}},
	} {
		messages := builder.BuildMessagesFromPrompt(req)
		if len(messages) < 2 || messages[0].Role != "system" {
			t.Fatal("missing system message")
		}
		if got := strings.Count(messages[0].Content, skills.SelfImprovementWorkflow); got != 1 {
			t.Fatalf("workflow included %d times, want once", got)
		}
	}
	if _, err := os.Stat(skills.SelfImprovementSkillPath(builder.workspace)); err != nil {
		t.Fatalf("skill not generated: %v", err)
	}
}

func TestSelfImprovementLearningRefreshesCachedPrompt(t *testing.T) {
	builder := NewContextBuilder(t.TempDir())
	before := builder.BuildSystemPromptWithCache()
	lesson := "When a test fails, reproduce it before changing the implementation."
	if err := builder.memory.AddEntry(MemoryTargetLearning, lesson); err != nil {
		t.Fatal(err)
	}
	after := builder.BuildSystemPromptWithCache()
	if before == after || !strings.Contains(after, "§ "+lesson) {
		t.Fatal("learned rule not included on next turn")
	}
	restarted := NewContextBuilder(builder.workspace)
	if !strings.Contains(restarted.BuildSystemPrompt(), lesson) {
		t.Fatal("rule lost on restart")
	}
}

func TestSelfImprovementRespectsExplicitSystemPromptOptOut(t *testing.T) {
	builder := NewContextBuilder(t.TempDir())
	messages := builder.BuildMessagesFromPrompt(PromptBuildRequest{
		CurrentMessage: "Hello", SuppressDefaultSystemPrompt: true,
	})
	for _, message := range messages {
		if strings.Contains(message.Content, skills.SelfImprovementWorkflow) {
			t.Fatal("ignored explicit system-prompt opt-out")
		}
	}
}
