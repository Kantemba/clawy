package plugin

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/pkg/plugins"
)

func newInstallCommand() *cobra.Command {
	var into string

	cmd := &cobra.Command{
		Use:   "install <path-or-git-url>",
		Short: "Install plugins from a directory or a git repository",
		Long: `Install plugins from a directory or a git repository.

The source may be a single plugin directory, a directory that contains
plugins (for example a clone of https://github.com/openai/plugins), or a git
URL which is cloned with --depth 1 before installation.

Installed plugins land in <workspace>/plugins unless --into is given.`,
		Example: `clawy plugin install ./notion
clawy plugin install ~/src/openai-plugins
clawy plugin install https://github.com/openai/plugins`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			dest := resolveDestination(cfg, into)
			installed, err := plugins.Install(args[0], dest)
			if err != nil {
				for _, dir := range installed {
					fmt.Fprintf(cmd.OutOrStdout(), "installed: %s\n", dir)
				}
				return err
			}

			out := cmd.OutOrStdout()
			for _, dir := range installed {
				fmt.Fprintf(out, "✓ installed %s\n", dir)
			}
			fmt.Fprintln(out, "\nRun `/reload` in a chat (or restart the gateway) to load the new plugins.")
			return nil
		},
	}

	cmd.Flags().StringVar(&into, "into", "", "Destination directory (default: <workspace>/plugins)")
	return cmd
}
