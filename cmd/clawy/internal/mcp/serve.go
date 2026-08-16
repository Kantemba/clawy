package mcp

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/pkg/mcpx/server"
)

func newServeCommand() *cobra.Command {
	var (
		transport string
		host      string
		port      int
		path      string
		enabled   bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the native Clawy MCP server (exposes Clawy tools to external MCP clients)",
		Long: `Start the native Clawy MCP server.

This runs Clawy itself as an MCP server, exposing Clawy's tools to external
MCP-compatible clients (e.g. Claude Desktop, Cursor).

The server can communicate via three transports:
  - stdio  : stdin/stdout JSON-RPC (suitable for subprocess invocation)
  - sse    : Server-Sent Events over HTTP
  - http   : Streamable HTTP (MCP 2025-03-26 spec)

The tool set is determined by the configured Clawy tools (enabled via
config). Tools that require a channel (e.g. send_message) will report
"mcp" as the channel.

Flags override config. When --transport is stdio the server reads from
stdin/stdout for the lifetime of the process.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			// Apply flag overrides onto config.
			if cmd.Flags().Changed("transport") {
				cfg.Tools.MCP.Native.Transport = transport
			}
			if cmd.Flags().Changed("host") {
				cfg.Tools.MCP.Native.Host = host
			}
			if cmd.Flags().Changed("port") {
				cfg.Tools.MCP.Native.Port = port
			}
			if cmd.Flags().Changed("path") {
				cfg.Tools.MCP.Native.Path = path
			}
			if cmd.Flags().Changed("enabled") {
				cfg.Tools.MCP.Native.Enabled = enabled
			}

			srv, err := mcpserver.NewServerFromConfig(cfg)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(),
				os.Interrupt, syscall.SIGTERM)
			defer stop()

			if err := srv.Start(ctx); err != nil {
				return fmt.Errorf("MCP server error: %w", err)
			}

			<-ctx.Done()
			stop()

			if err := srv.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "MCP server shutdown error: %s\n", err)
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "MCP server stopped")
			return nil
		},
	}

	cmd.Flags().StringVar(&transport, "transport", "", "Transport type: stdio|http|sse (default: native from config)")
	cmd.Flags().StringVar(&host, "host", "", "Bind HTTP host (sse/http transports)")
	cmd.Flags().IntVar(&port, "port", 0, "Bind HTTP port (sse/http transports)")
	cmd.Flags().StringVar(&path, "path", "", "HTTP path for SSE/HTTP transports")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Enable the native MCP server")

	return cmd
}
