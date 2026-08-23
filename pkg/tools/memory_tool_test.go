package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kantemba/clawy/pkg/agent"
	"github.com/Kantemba/clawy/pkg/tools"
)

// memoryToolHarness wires the real agent.MemoryStore as backend, exactly like
// production registration does.
func memoryToolHarness(t *testing.T) (*tools.MemoryTool, string) {
	t.Helper()
	ws := t.TempDir()
	store := agent.NewMemoryStore(ws)
	return tools.NewMemoryTool(store), filepath.Join(ws, "memory", "MEMORY.md")
}

func TestMemoryToolAddPersistsEntry(t *testing.T) {
	tool, memoryPath := memoryToolHarness(t)

	res := tool.Execute(context.Background(), map[string]any{
		"action": "add",
		"target": "memory",
		"text":   "Prefers dark mode.",
	})
	if res.IsError {
		t.Fatalf("add failed: %s", res.ForLLM)
	}

	data, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "§ Prefers dark mode.") {
		t.Fatalf("entry not persisted to %s: %q", memoryPath, string(data))
	}
	if strings.Contains(res.ForLLM, "§") == false && !strings.Contains(res.ForLLM, "Added to") {
		t.Fatalf("unexpected result: %q", res.ForLLM)
	}
}

func TestMemoryToolValidationAndLifecycle(t *testing.T) {
	tool, _ := memoryToolHarness(t)
	ctx := context.Background()

	if res := tool.Execute(ctx, map[string]any{"action": "add", "target": "memory"}); !res.IsError {
		t.Fatal("add without text must fail")
	}
	if res := tool.Execute(ctx, map[string]any{"action": "add", "target": "bogus", "text": "x"}); !res.IsError {
		t.Fatal("unknown target must fail")
	}
	if res := tool.Execute(ctx, map[string]any{"action": "bogus", "target": "memory"}); !res.IsError {
		t.Fatal("unknown action must fail")
	}
	if res := tool.Execute(ctx, map[string]any{"action": "replace", "target": "memory", "old_text": "nope"}); !res.IsError {
		t.Fatal("replace of missing old_text must fail")
	}

	// Full lifecycle: add → read → replace → read → remove → read.
	tool.Execute(ctx, map[string]any{"action": "add", "target": "user", "text": "Works at Acme."})
	res := tool.Execute(ctx, map[string]any{"action": "read", "target": "user"})
	if res.IsError || !strings.Contains(res.ForLLM, "§ Works at Acme.") {
		t.Fatalf("read should list entries: %q", res.ForLLM)
	}

	tool.Execute(ctx, map[string]any{"action": "replace", "target": "user", "old_text": "Acme", "new_text": "Globex"})
	res = tool.Execute(ctx, map[string]any{"action": "read", "target": "user"})
	if !strings.Contains(res.ForLLM, "Globex") {
		t.Fatalf("replace did not apply: %q", res.ForLLM)
	}

	if res := tool.Execute(ctx, map[string]any{"action": "remove", "target": "user", "old_text": "Globex"}); res.IsError {
		t.Fatalf("remove failed: %s", res.ForLLM)
	}
	res = tool.Execute(ctx, map[string]any{"action": "read", "target": "user"})
	if !strings.Contains(res.ForLLM, "empty") {
		t.Fatalf("expected empty notice after removal: %q", res.ForLLM)
	}
}

func TestMemoryToolSurfacesConsolidationErrors(t *testing.T) {
	tool, _ := memoryToolHarness(t)
	ctx := context.Background()

	big := strings.Repeat("x", 1400) // USER budget is 1375
	res := tool.Execute(ctx, map[string]any{"action": "add", "target": "user", "text": big})
	if !res.IsError || !strings.Contains(res.ForLLM, "Consolidate first") {
		t.Fatalf("capacity error should surface consolidation guidance: %q", res.ForLLM)
	}
}

