package version

import (
	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/cmd/clawy/internal"
	"github.com/Kantemba/clawy/cmd/clawy/internal/cliui"
	"github.com/Kantemba/clawy/pkg/config"
)

func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "Show version information",
		Run: func(_ *cobra.Command, _ []string) {
			printVersion()
		},
	}

	return cmd
}

func printVersion() {
	build, goVer := config.FormatBuildInfo()
	cliui.PrintVersion(internal.Logo, "clawy "+config.FormatVersion(), build, goVer)
}
