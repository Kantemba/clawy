package start

import (
	"context"
	"strconv"

	"github.com/spf13/cobra"

	webconsole "github.com/Kantemba/clawy/clients/web/backend"
)

var runWebConsole = webconsole.Run

// RunDefault supports opening Clawy by double-clicking the same executable.
func RunDefault(ctx context.Context) error {
	return runWebConsole(ctx, webconsole.Options{Console: true})
}

// NewStartCommand serves the web console directly from the Clawy executable.
func NewStartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "start [config.json]",
		Short:   "Open the web console to set up and run Clawy",
		Long:    "Start the embedded web console and open your browser. Configure models, credentials, and channels there; no other binary or CLI onboarding is required. Keep this terminal open while using Clawy.",
		Example: "clawy start\nclawy start --no-browser\nclawy start --port 19999 ./config.json",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWebConsole(cmd.Context(), optionsFor(cmd, args))
		},
	}
	cmd.Flags().Int("port", 18800, "Web console port (defaults to saved settings)")
	cmd.Flags().String("host", "", "Host to listen on (default: localhost)")
	cmd.Flags().Bool("public", false, "Allow access from other devices")
	cmd.Flags().Bool("no-browser", false, "Do not open the browser automatically")
	cmd.Flags().BoolP("debug", "d", false, "Enable debug logging")
	cmd.Flags().String("lang", "", "Language: en or zh (default: system locale)")
	return cmd
}

func optionsFor(cmd *cobra.Command, args []string) webconsole.Options {
	port, _ := cmd.Flags().GetInt("port")
	host, _ := cmd.Flags().GetString("host")
	public, _ := cmd.Flags().GetBool("public")
	noBrowser, _ := cmd.Flags().GetBool("no-browser")
	debug, _ := cmd.Flags().GetBool("debug")
	lang, _ := cmd.Flags().GetString("lang")
	opts := webconsole.Options{
		Port: strconv.Itoa(port), Host: host, Public: public,
		NoBrowser: noBrowser, Debug: debug, Language: lang, Console: true,
		PortSet:   cmd.Flags().Changed("port"),
		HostSet:   cmd.Flags().Changed("host"),
		PublicSet: cmd.Flags().Changed("public"),
	}
	if len(args) > 0 {
		opts.ConfigPath = args[0]
	}
	return opts
}
