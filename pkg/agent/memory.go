package agent

import (
	"strings"

	"github.com/Kantemba/clawy/pkg/logger"
	"github.com/Kantemba/clawy/pkg/memory"
)

// Compatibility aliases keep existing tool integrations and workspaces intact.
// The implementation is shared with evolution so background learning and
// foreground memory edits use the same validation, budgets and workspace lock.
type MemoryStore = memory.CuratedStore

const (
	MemoryTargetAgent           = memory.TargetAgent
	MemoryTargetUser            = memory.TargetUser
	MemoryTargetLearning        = memory.TargetLearning
	DefaultAgentMemoryBudget    = memory.AgentBudget
	DefaultUserMemoryBudget     = memory.UserBudget
	DefaultLearningMemoryBudget = memory.LearningBudget
	entryMarker                 = memory.EntryMarker
)

func NewMemoryStore(workspace string) *MemoryStore {
	store := memory.NewCuratedStore(workspace)
	// An unset workspace (e.g. a tooling-only instance) must not create a
	// persistent skill in the process working directory.
	if strings.TrimSpace(workspace) != "" {
		if err := store.EnsureSelfImprovementSkill(); err != nil {
			logger.WarnCF("agent", "Could not initialize self-improvement skill", map[string]any{"error": err.Error()})
		}
	}
	return store
}

func ParseEntries(raw string) []string {
	return memory.ParseCuratedEntries(raw)
}
