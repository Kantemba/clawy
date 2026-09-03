package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/cmd/clawy/internal"
	"github.com/Kantemba/clawy/cmd/clawy/internal/cliui"
	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/updater"
)

func NewVersionCommand() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "Show version information",
		Run: func(_ *cobra.Command, _ []string) {
			printVersion()
			if check {
				st, err := updater.CheckForUpdate(config.GetVersion())
				if err != nil {
					fmt.Printf("Update check failed: %v\n", err)
					return
				}
				if notice := updater.FormatUpdateNotice(st); notice != "" {
					fmt.Println(notice)
				} else {
					fmt.Printf("Up to date (latest: %s).\n", st.Latest.TagName)
				}
			}
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Also check GitHub releases for a newer version")

	return cmd
}

func printVersion() {
	build, goVer := config.FormatBuildInfo()
	cliui.PrintVersion(internal.Logo, "clawy "+config.FormatVersion(), build, goVer)
}
