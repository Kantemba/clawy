package plugin

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/pkg/plugins"
)

func newListCommand() *cobra.Command {
	var showRoots bool

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List discovered plugins",
		Example: `clawy plugin list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			bundle := plugins.Discover(cfg)
			out := cmd.OutOrStdout()

			if bundle.IsEmpty() {
				fmt.Fprintln(out, "No plugins found.")
			} else {
				writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
				fmt.Fprintln(writer, "NAME\tVERSION\tFORMAT\tSURFACES\tDIR")
				for _, plugin := range bundle.Plugins {
					fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
						plugin.Name(),
						plugin.Version(),
						plugin.Format(),
						strings.Join(plugin.Surfaces(), " "),
						plugin.Dir,
					)
				}
				_ = writer.Flush()
			}

			for _, warning := range bundle.Warnings {
				fmt.Fprintf(out, "⚠ %s\n", warning)
			}

			if showRoots || bundle.IsEmpty() {
				fmt.Fprintln(out, "\nSearch roots:")
				for _, root := range plugins.SearchRoots(cfg) {
					fmt.Fprintf(out, "  %s\n", root)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&showRoots, "roots", false, "Also print the plugin search roots")
	return cmd
}
