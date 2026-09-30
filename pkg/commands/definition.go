package commands

import (
	"fmt"
	"strings"
)

// SubCommand defines a single sub-command within a parent command.
type SubCommand struct {
	Name        string
	Description string
	ArgsUsage   string // optional, e.g. "<session-id>"
	Handler     Handler
}

// Definition is the single-source metadata and behavior contract for a slash command.
//
// Design notes (phase 1):
//   - Every channel reads command shape from this type instead of keeping local copies.
//   - Visibility is global: all definitions are considered available to all channels.
//   - Platform menu registration (for example Telegram BotCommand) also derives from this
//     same definition so UI labels and runtime behavior stay aligned.
type Definition struct {
	Name        string
	Description string
	Usage       string // for simple commands; ignored when SubCommands is set
	Aliases     []string
	SubCommands []SubCommand // optional; when set, Executor routes to sub-command handlers
	Handler     Handler      // for simple commands without sub-commands

	// Prompt, when set, marks this definition as a prompt command: the markdown
	// body is expanded with the message arguments and forwarded to the LLM
	// instead of running a Go handler. This is how plugin commands
	// (commands/*.md) are implemented; builtins leave it empty.
	Prompt string
}

// ExpandPrompt renders a prompt command body with the arguments that followed
// the command name.
//
// Supported placeholders:
//
//	{{args}} / $ARGUMENTS — the full argument string
//	$1 .. $9              — individual argument tokens
//
// When the body contains no placeholder, the arguments are appended as a
// final paragraph.
func (d Definition) ExpandPrompt(args string) string {
	args = strings.TrimSpace(args)
	body := strings.TrimSpace(d.Prompt)
	if body == "" {
		return ""
	}

	fields := strings.Fields(args)
	expanded := strings.ReplaceAll(body, "{{args}}", args)
	expanded = strings.ReplaceAll(expanded, "$ARGUMENTS", args)
	for i := 1; i <= 9; i++ {
		value := ""
		if i <= len(fields) {
			value = fields[i-1]
		}
		expanded = strings.ReplaceAll(expanded, fmt.Sprintf("$%d", i), value)
	}

	if args == "" || strings.Contains(body, "{{args}}") || strings.Contains(body, "$ARGUMENTS") ||
		containsPositionalPlaceholder(body) {
		return strings.TrimSpace(expanded)
	}
	return strings.TrimSpace(expanded) + "\n\n" + args
}

// containsPositionalPlaceholder reports whether the body references $1..$9.
func containsPositionalPlaceholder(body string) bool {
	for i := 1; i <= 9; i++ {
		if strings.Contains(body, fmt.Sprintf("$%d", i)) {
			return true
		}
	}
	return false
}

// EffectiveUsage returns the usage string. When SubCommands are present,
// it is auto-generated from sub-command names so metadata and behavior
// cannot drift.
func (d Definition) EffectiveUsage() string {
	if len(d.SubCommands) == 0 {
		return d.Usage
	}
	names := make([]string, 0, len(d.SubCommands))
	for _, sc := range d.SubCommands {
		name := sc.Name
		if sc.ArgsUsage != "" {
			name += " " + sc.ArgsUsage
		}
		names = append(names, name)
	}
	return fmt.Sprintf("/%s [%s]", d.Name, strings.Join(names, "|"))
}
