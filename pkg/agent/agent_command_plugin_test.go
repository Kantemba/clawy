package agent

import (
	"testing"

	"github.com/Kantemba/clawy/pkg/commands"
)

func newAgentLoopWithPluginCommand(prompt string) *AgentLoop {
	al := &AgentLoop{cmdRegistry: commands.NewRegistry(commands.BuiltinDefinitions())}
	al.cmdRegistry.SetExtras([]commands.Definition{
		{Name: "deploy", Description: "plugin command", Prompt: prompt},
	})
	return al
}

func TestExpandPromptCommandRewritesMessage(t *testing.T) {
	al := newAgentLoopWithPluginCommand("Deploy {{args}} to production")
	opts := &processOptions{}

	if !al.expandPromptCommand("/deploy api v2", opts) {
		t.Fatal("expandPromptCommand() = false, want true for a plugin prompt command")
	}
	if want := "Deploy api v2 to production"; opts.UserMessage != want {
		t.Errorf("UserMessage = %q, want %q", opts.UserMessage, want)
	}
	if opts.Dispatch.UserMessage != opts.UserMessage {
		t.Errorf("Dispatch.UserMessage = %q, want %q", opts.Dispatch.UserMessage, opts.UserMessage)
	}
}

func TestExpandPromptCommandIgnoresBuiltinsAndPlainText(t *testing.T) {
	al := newAgentLoopWithPluginCommand("Deploy {{args}}")

	builtin := &processOptions{}
	if al.expandPromptCommand("/help", builtin) {
		t.Error("builtin commands have no prompt body and must not be rewritten")
	}
	if builtin.UserMessage != "" {
		t.Errorf("UserMessage = %q, want untouched", builtin.UserMessage)
	}

	plain := &processOptions{}
	if al.expandPromptCommand("just a normal message", plain) {
		t.Error("plain text must not be treated as a plugin command")
	}
}

func TestExpandPromptCommandWithoutOptionsIsPassthrough(t *testing.T) {
	al := newAgentLoopWithPluginCommand("Deploy {{args}}")

	if al.expandPromptCommand("/deploy api", nil) {
		t.Error("expandPromptCommand(nil opts) = true, want false (cannot rewrite)")
	}
}
