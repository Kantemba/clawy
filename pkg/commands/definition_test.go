package commands

import (
	"testing"
)

func TestDefinition_EffectiveUsage_NoSubCommands(t *testing.T) {
	d := Definition{Name: "start", Usage: "/start"}
	if got := d.EffectiveUsage(); got != "/start" {
		t.Fatalf("EffectiveUsage()=%q, want %q", got, "/start")
	}
}

func TestDefinition_EffectiveUsage_WithSubCommands(t *testing.T) {
	d := Definition{
		Name: "show",
		SubCommands: []SubCommand{
			{Name: "model"},
			{Name: "channel"},
			{Name: "agents"},
		},
	}
	want := "/show [model|channel|agents]"
	if got := d.EffectiveUsage(); got != want {
		t.Fatalf("EffectiveUsage()=%q, want %q", got, want)
	}
}

func TestDefinition_EffectiveUsage_WithArgsUsage(t *testing.T) {
	d := Definition{
		Name: "session",
		SubCommands: []SubCommand{
			{Name: "list"},
			{Name: "resume", ArgsUsage: "<id>"},
		},
	}
	want := "/session [list|resume <id>]"
	if got := d.EffectiveUsage(); got != want {
		t.Fatalf("EffectiveUsage()=%q, want %q", got, want)
	}
}

func TestDefinition_ExpandPrompt(t *testing.T) {
	cases := []struct {
		name string
		body string
		args string
		want string
	}{
		{"placeholder", "Deploy {{args}} now", "api v2", "Deploy api v2 now"},
		{"dollar-args", "Deploy $ARGUMENTS", "api v2", "Deploy api v2"},
		{"positional", "Deploy $1 to $2", "api prod", "Deploy api to prod"},
		{"append-when-no-placeholder", "Deploy the service", "api", "Deploy the service\n\napi"},
		{"missing-positional-becomes-empty", "Deploy $1", "", "Deploy"},
		{"no-args-with-placeholder", "Say {{args}}", "", "Say"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := Definition{Name: "deploy", Prompt: tc.body}
			if got := def.ExpandPrompt(tc.args); got != tc.want {
				t.Errorf("ExpandPrompt(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}
