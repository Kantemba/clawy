package plugin

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/pkg/plugins"
)

func newNewCommand() *cobra.Command {
	var (
		into        string
		description string
	)

	cmd := &cobra.Command{
		Use:     "new <name>",
		Short:   "Scaffold a new Clawy plugin",
		Example: `clawy plugin new my-plugin --description "Does something useful"`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			dest := resolveDestination(cfg, into)
			result, err := plugins.Scaffold(dest, args[0], description)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "✓ created plugin at %s\n\n", result.Dir)
			for _, file := range result.Files {
				fmt.Fprintf(out, "  %s\n", file)
			}
			fmt.Fprintln(out, "\nEdit the files, then run `clawy plugin list` to verify discovery.")
			return nil
		},
	}

	cmd.Flags().StringVar(&into, "into", "", "Destination directory (default: <workspace>/plugins)")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Plugin description")
	return cmd
}
