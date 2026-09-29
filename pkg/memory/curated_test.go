package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Kantemba/clawy/pkg/skills"
)

func TestCuratedLearningSkillLifecycle(t *testing.T) {
	workspace := t.TempDir()
	store := NewCuratedStore(workspace)
	if err := store.EnsureSelfImprovementSkill(); err != nil {
		t.Fatal(err)
	}
	lesson := "When fixing a regression, add a reproducing test and verify it passes."
	if err := store.AddEntry(TargetLearning, lesson); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(skills.SelfImprovementSkillPath(workspace))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: self-improvement", skills.SelfImprovementWorkflow, EntryMarker + " " + lesson} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("skill missing %q", want)
		}
	}
	restarted := NewCuratedStore(workspace)
	if err := restarted.EnsureSelfImprovementSkill(); err != nil {
		t.Fatal(err)
	}
	if got := restarted.ReadEntries(TargetLearning); len(got) != 1 || got[0] != lesson {
		t.Fatalf("lost rules: %v", got)
	}
	if err := restarted.ReplaceEntry(TargetLearning, "regression", "parser regression"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restarted.SelfImprovementContext(), "parser regression") {
		t.Fatal("updated rule not loaded")
	}
	if err := restarted.RemoveEntry(TargetLearning, "parser regression"); err != nil {
		t.Fatal(err)
	}
	if len(restarted.ReadEntries(TargetLearning)) != 0 {
		t.Fatal("rule not removed")
	}
	if !strings.Contains(restarted.SelfImprovementContext(), skills.SelfImprovementWorkflow) {
		t.Fatal("workflow was removed")
	}
}

func TestCuratedStorePreservesLegacyFacts(t *testing.T) {
	workspace := t.TempDir()
	store := NewCuratedStore(workspace)
	legacy := "# User\n\nPrefers concise answers.\n"
	if err := os.WriteFile(store.Path(TargetUser), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureSelfImprovementSkill(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.Path(TargetUser))
	if err != nil || string(data) != legacy {
		t.Fatal("initialization changed legacy memory")
	}
	if err := store.AddEntry(TargetUser, "Works on embedded devices."); err != nil {
		t.Fatal(err)
	}
	if got := store.ReadEntries(TargetUser); len(got) != 2 || !strings.Contains(got[0], "Prefers concise") {
		t.Fatalf("legacy facts lost: %v", got)
	}
}

func TestCuratedStoreConcurrentInstancesDoNotLoseUpdates(t *testing.T) {
	workspace := t.TempDir()
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store := NewCuratedStore(workspace)
			if err := store.AddEntry(TargetLearning, fmt.Sprintf("When checking component [%02d], run its regression suite.", i)); err != nil {
				t.Errorf("add: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := NewCuratedStore(workspace).ReadEntries(TargetLearning); len(got) != 12 {
		t.Fatalf("lost updates: %v", got)
	}
}

func TestCuratedStoreRejectsMalformedLearningWithoutOverwriting(t *testing.T) {
	store := NewCuratedStore(t.TempDir())
	path := store.Path(TargetLearning)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "# My existing self-improvement skill\nDo not destroy this.\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureSelfImprovementSkill(); err == nil {
		t.Fatal("expected malformed skill error")
	}
	if err := store.AddEntry(TargetLearning, "Run tests."); err == nil {
		t.Fatal("must not overwrite malformed skill")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("original skill destroyed")
	}
	if !strings.Contains(store.SelfImprovementContext(), skills.SelfImprovementWorkflow) {
		t.Fatal("missing fallback workflow")
	}
}

func TestCuratedStoreValidation(t *testing.T) {
	store := NewCuratedStore(t.TempDir())
	for _, target := range []string{TargetAgent, TargetUser, TargetLearning} {
		for _, text := range []string{"password: dont-echo-me", "a\n§ injected entry", skills.SelfImprovementLessonsEnd, "bad\x00text"} {
			err := store.AddEntry(target, text)
			if err == nil {
				t.Fatalf("accepted invalid entry %q for %s", text, target)
			}
			if strings.Contains(err.Error(), "dont-echo-me") {
				t.Fatal("secret leaked in error")
			}
		}
		if len(store.ReadEntries(target)) != 0 {
			t.Fatal("invalid entries persisted")
		}
	}
	if err := store.AddEntry(TargetLearning, strings.Repeat("界", LearningBudget)); err == nil || !strings.Contains(err.Error(), "Consolidate first") {
		t.Fatalf("expected Unicode budget error: %v", err)
	}
}

func TestCuratedStoreReplacementRejectsAmbiguityDuplicatesAndSecrets(t *testing.T) {
	store := NewCuratedStore(t.TempDir())
	for _, fact := range []string{"Project alpha uses Go.", "Project beta uses Rust."} {
		if err := store.AddEntry(TargetAgent, fact); err != nil {
			t.Fatal(err)
		}
	}
	for _, change := range [][2]string{{"Project", "Tool"}, {"Project beta uses Rust.", "Project alpha uses Go."}, {"Go", "password: hidden"}} {
		if err := store.ReplaceEntry(TargetAgent, change[0], change[1]); err == nil {
			t.Fatalf("accepted invalid replacement: %v", change)
		}
	}
	if got := store.ReadEntries(TargetAgent); len(got) != 2 || got[0] != "Project alpha uses Go." {
		t.Fatalf("changed entries on failed write: %v", got)
	}
}

func TestCuratedStoreDoesNotTreatReadFailureAsEmpty(t *testing.T) {
	store := NewCuratedStore(t.TempDir())
	if err := os.MkdirAll(store.Path(TargetAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.AddEntry(TargetAgent, "A durable fact."); err == nil {
		t.Fatal("must report read failure")
	}
	if _, err := store.ReadEntriesWithError(TargetAgent); err == nil {
		t.Fatal("must report read failure")
	}
}
