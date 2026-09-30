// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/cmd/clawy/internal"
	"github.com/Kantemba/clawy/cmd/clawy/internal/agent"
	"github.com/Kantemba/clawy/cmd/clawy/internal/auth"
	"github.com/Kantemba/clawy/cmd/clawy/internal/cliui"
	configcmd "github.com/Kantemba/clawy/cmd/clawy/internal/config"
	"github.com/Kantemba/clawy/cmd/clawy/internal/cron"
	"github.com/Kantemba/clawy/cmd/clawy/internal/gateway"
	"github.com/Kantemba/clawy/cmd/clawy/internal/mcp"
	"github.com/Kantemba/clawy/cmd/clawy/internal/migrate"
	"github.com/Kantemba/clawy/cmd/clawy/internal/model"
	"github.com/Kantemba/clawy/cmd/clawy/internal/onboard"
	pairingcmd "github.com/Kantemba/clawy/cmd/clawy/internal/pairing"
	plugincmd "github.com/Kantemba/clawy/cmd/clawy/internal/plugin"
	"github.com/Kantemba/clawy/cmd/clawy/internal/skills"
	"github.com/Kantemba/clawy/cmd/clawy/internal/status"
	"github.com/Kantemba/clawy/cmd/clawy/internal/version"
	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/updater"
)

var rootNoColor bool

// initTermuxSSL detects Termux environment and sets SSL_CERT_FILE if not already set.
// This fixes X509 certificate errors when running Clawy inside Termux or termux-chroot.
// See: https://github.com/Kantemba/clawy/issues/2944
func initTermuxSSL() {
	// Only applicable on Linux/Android
	if runtime.GOOS != "linux" && runtime.GOOS != "android" {
		return
	}

	// Skip if already set
	if os.Getenv("SSL_CERT_FILE") != "" {
		return
	}

	// Check for Termux prefix in PATH or HOME
	home := os.Getenv("HOME")
	path := os.Getenv("PATH")

	isTermux := strings.Contains(home, "com.termux") ||
		strings.Contains(path, "com.termux") ||
		strings.Contains(home, "/data/data/com.termux")

	if !isTermux {
		return
	}

	// Check common CA bundle locations in Termux
	caPaths := []string{
		"$PREFIX/etc/tls/cert.pem",
		os.Getenv("PREFIX") + "/etc/tls/cert.pem",
		"/data/data/com.termux/files/usr/etc/tls/cert.pem",
		"/usr/etc/tls/cert.pem",
	}

	for _, caPath := range caPaths {
		expanded := os.ExpandEnv(caPath)
		if _, err := os.Stat(expanded); err == nil {
			os.Setenv("SSL_CERT_FILE", expanded)
			return
		}
	}
}

func syncCliUIColor(root *cobra.Command) {
	no, _ := root.PersistentFlags().GetBool("no-color")
	cliui.Init(no || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb")
}

// earlyColorDisabled matches lipgloss/banner behavior from env and argv before Cobra parses flags.
func earlyColorDisabled() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return true
	}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--no-color" || arg == "--no-color=true" || arg == "--no-color=1" {
			return true
		}
	}
	return false
}

func NewClawyCommand() *cobra.Command {
	short := fmt.Sprintf("%s Clawy — personal AI assistant", internal.Logo)
	long := fmt.Sprintf(`%s Clawy is a lightweight personal AI assistant.

Version: %s`, internal.Logo, config.FormatVersion())

	cmd := &cobra.Command{
		Use:   "clawy",
		Short: short,
		Long:  long,
		Example: `clawy version
clawy onboard
clawy --no-color status`,
		SilenceErrors: true,
		// Avoid plain UsageString() on stderr/stdout when a command fails; cliui
		// renders matching panels on stderr instead.
		SilenceUsage: true,
		PersistentPreRun: func(c *cobra.Command, _ []string) {
			syncCliUIColor(c.Root())
		},
	}

	cmd.PersistentFlags().BoolVar(&rootNoColor, "no-color", false,
		"Disable colors (boxed layout unchanged)")

	cmd.SetHelpFunc(func(c *cobra.Command, _ []string) {
		syncCliUIColor(c.Root())
		fmt.Fprint(c.OutOrStdout(), cliui.RenderCommandHelp(c))
	})

	cmd.AddCommand(
		configcmd.NewConfigCommand(),
		onboard.NewOnboardCommand(),
		agent.NewAgentCommand(),
		auth.NewAuthCommand(),
		gateway.NewGatewayCommand(),
		status.NewStatusCommand(),
		cron.NewCronCommand(),
		mcp.NewMCPCommand(),
		plugincmd.NewPluginCommand(),
		migrate.NewMigrateCommand(),
		skills.NewSkillsCommand(),
		model.NewModelCommand(),
		pairingcmd.NewPairingCommand(),
		updater.NewUpdateCommand("clawy"),
		version.NewVersionCommand(),
	)

	return cmd
}

func main() {
	// Initialize Termux SSL certificate detection before anything else
	initTermuxSSL()

	cliui.Init(earlyColorDisabled())

	fmt.Printf("Clawy %s\n", config.FormatVersion())

	// Non-blocking update notification: show the cached result (if any)
	// and refresh the cache in the background for the next run.
	// Never blocks startup; disable with CLAWY_NO_UPDATE_CHECK=1.
	if notice, ok := updater.CachedUpdateNotice(); ok && notice != "" {
		fmt.Fprintln(os.Stderr, notice)
	}
	go updater.RefreshUpdateCache()

	tzEnv := os.Getenv("TZ")
	if tzEnv != "" {
		fmt.Println("TZ environment:", tzEnv)
		zoneinfoEnv := os.Getenv("ZONEINFO")
		fmt.Println("ZONEINFO environment:", zoneinfoEnv)
		loc, err := time.LoadLocation(tzEnv)
		if err != nil {
			fmt.Println("Error loading time zone:", err)
		} else {
			fmt.Println("Time zone loaded successfully:", loc)
			time.Local = loc //nolint:gosmopolitan // We intentionally set local timezone from TZ env
		}
	}

	cmd := NewClawyCommand()
	last, err := cmd.ExecuteC()
	if err != nil {
		syncCliUIColor(cmd)
		fmt.Fprint(os.Stderr, cliui.FormatCLIError(err.Error(), last))
		os.Exit(1)
	}
}
