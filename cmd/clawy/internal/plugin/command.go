package plugin

import (
	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/cmd/clawy/internal"
	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/plugins"
)

// NewPluginCommand manages directory-based plugins (skills, commands, MCP
// servers and hooks contributed by a plugin directory).
func NewPluginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage plugins",
		Long: `Manage plugins.

A plugin is a directory with a manifest. Two dialects are supported:

  .codex-plugin/plugin.json   Codex layout (https://github.com/openai/plugins)
  clawy-plugin.json           Clawy native layout

Search roots (in order):

  $CLAWY_PLUGINS, config plugins.dirs, <workspace>/plugins, ~/.clawy/plugins

Surfaces a plugin may provide:

  skills/     skills/<name>/SKILL.md   injected into the skill catalog
  commands/   commands/<name>.md       slash prompt commands
  .mcp.json   {"mcpServers": {...}}    MCP servers connected at startup
  hooks.json  processes/builtins       mounted by the hook runtime`,
		Example: `clawy plugin list
clawy plugin info notion
clawy plugin new my-plugin
clawy plugin install ./openai-plugins
clawy plugin install https://github.com/openai/plugins`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newListCommand(),
		newInfoCommand(),
		newInstallCommand(),
		newNewCommand(),
	)

	return cmd
}

// loadConfig is shared by the plugin subcommands.
func loadConfig() (*config.Config, error) {
	cfg, err := internal.LoadConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// resolveDestination returns the directory plugins are installed into.
func resolveDestination(cfg *config.Config, override string) string {
	if override != "" {
		return override
	}
	return plugins.DestinationRoot(cfg)
}
