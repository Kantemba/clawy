package commands

import "testing"

func TestRegistry_Definitions_ReturnsCopy(t *testing.T) {
	defs := []Definition{
		{Name: "help", Description: "Show help"},
		{Name: "admin", Description: "Admin command"},
	}
	r := NewRegistry(defs)

	got := r.Definitions()
	if len(got) != 2 {
		t.Fatalf("definitions len = %d, want 2", len(got))
	}

	got[0].Name = "mutated"
	again := r.Definitions()
	if again[0].Name != "help" {
		t.Fatalf("registry should not be mutated by caller, got first name %q", again[0].Name)
	}
}

func TestRegistry_Lookup_MatchesByLowercaseNameAndAlias(t *testing.T) {
	r := NewRegistry([]Definition{
		{Name: "Help", Aliases: []string{"Assist"}},
		{Name: "List"},
	})

	def, ok := r.Lookup("help")
	if !ok || def.Name != "Help" {
		t.Fatalf("lookup by lowercase name failed: ok=%v def=%+v", ok, def)
	}

	def, ok = r.Lookup("HELP")
	if !ok || def.Name != "Help" {
		t.Fatalf("lookup by uppercase name failed: ok=%v def=%+v", ok, def)
	}

	def, ok = r.Lookup("assist")
	if !ok || def.Name != "Help" {
		t.Fatalf("lookup by lowercase alias failed: ok=%v def=%+v", ok, def)
	}

	def, ok = r.Lookup("ASSIST")
	if !ok || def.Name != "Help" {
		t.Fatalf("lookup by uppercase alias failed: ok=%v def=%+v", ok, def)
	}
}

func TestRegistry_SetExtras_PluginsOnTopOfBase(t *testing.T) {
	r := NewRegistry([]Definition{
		{Name: "help", Description: "builtin"},
		{Name: "list"},
	})

	added := r.SetExtras([]Definition{
		{Name: "deploy", Description: "plugin command", Prompt: "deploy {{args}}"},
		{Name: "help", Description: "plugin tries to shadow a builtin"},
	})
	if added != 1 {
		t.Fatalf("SetExtras() added = %d, want 1 (builtin names are skipped)", added)
	}

	if len(r.Definitions()) != 3 {
		t.Fatalf("definitions len = %d, want 3", len(r.Definitions()))
	}

	def, ok := r.Lookup("help")
	if !ok || def.Description != "builtin" {
		t.Errorf("builtin definition must win, got %+v", def)
	}
	def, ok = r.Lookup("deploy")
	if !ok || def.Prompt == "" {
		t.Errorf("plugin definition missing, got %+v", def)
	}
}

func TestRegistry_SetExtras_ReplacesPreviousExtras(t *testing.T) {
	r := NewRegistry([]Definition{{Name: "help"}})
	r.SetExtras([]Definition{{Name: "first", Prompt: "one"}})

	r.SetExtras([]Definition{{Name: "second", Prompt: "two"}})

	if _, ok := r.Lookup("first"); ok {
		t.Error("stale plugin command should be removed by SetExtras")
	}
	if def, ok := r.Lookup("second"); !ok || def.Prompt != "two" {
		t.Errorf("second = %+v, want registered", def)
	}
	if _, ok := r.Lookup("help"); !ok {
		t.Error("builtin command disappeared")
	}
	if len(r.Definitions()) != 2 {
		t.Errorf("definitions len = %d, want 2", len(r.Definitions()))
	}
}

func TestRegistry_Add_IsIdempotent(t *testing.T) {
	r := NewRegistry([]Definition{{Name: "help"}})

	if added := r.Add(Definition{Name: "deploy", Prompt: "x"}); added != 1 {
		t.Fatalf("Add() added = %d, want 1", added)
	}
	if added := r.Add(Definition{Name: "deploy", Prompt: "y"}); added != 0 {
		t.Fatalf("second Add() added = %d, want 0", added)
	}
	if len(r.Definitions()) != 2 {
		t.Errorf("definitions len = %d, want 2", len(r.Definitions()))
	}
}
