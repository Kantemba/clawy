package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/pkg/config"
	picomcp "github.com/Kantemba/clawy/pkg/mcp"
)

// Indirection points so tests can stub the OAuth flows.
var (
	mcpLoginFunc   = picomcp.LoginMCPServer
	mcpLogoutFunc  = picomcp.LogoutMCPServer
	mcpHasCredFunc = picomcp.HasStoredMCPCredential
)

func newAuthCommand() *cobra.Command {
	var (
		timeout      time.Duration
		noBrowser    bool
		clientID     string
		clientSecret string
		scopes       []string
		issuer       string
		port         int
		redirectPath string
	)

	cmd := &cobra.Command{
		Use:   "auth <name>",
		Short: "Authenticate a remote MCP server using OAuth",
		Long: "Runs the OAuth authorization-code flow (with PKCE) for a remote (sse/http) MCP server.\n" +
			"When the callback URL is on localhost, Clawy starts a local listener and ALSO lets you\n" +
			"paste the full callback URL from your browser into the terminal - whichever happens\n" +
			"first wins. Tokens are stored encrypted and refreshed automatically.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			name := args[0]
			server, exists := cfg.Tools.MCP.Servers[name]
			if !exists {
				return fmt.Errorf("MCP server %q not found", name)
			}
			if server.URL == "" {
				return fmt.Errorf(
					"MCP server %q uses the stdio transport; OAuth applies to remote (sse/http) servers only",
					name)
			}

			oauthCfg := ensureOAuthConfig(server.OAuth)
			applyAuthFlags(oauthCfg, clientID, clientSecret, scopes, issuer, port, redirectPath, noBrowser)
			server.OAuth = oauthCfg

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			fmt.Fprintf(cmd.OutOrStdout(), "Starting OAuth authentication for MCP server %q...\n", name)
			if loginErr := mcpLoginFunc(ctx, name, server); loginErr != nil {
				return loginErr
			}

			cfg.Tools.MCP.Servers[name] = server
			if saveErr := saveValidatedConfig(cfg); saveErr != nil {
				return fmt.Errorf(
					"authenticated successfully, but failed to persist the oauth configuration: %w",
					saveErr)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✓ MCP server %q authenticated.\n", name)
			return nil
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "How long to wait for authorization to complete")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Do not open the browser automatically")
	cmd.Flags().StringVar(&clientID, "client-id", "", "Pre-registered OAuth client id")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "OAuth client secret for confidential clients")
	cmd.Flags().StringSliceVar(&scopes, "scopes", nil, "OAuth scopes to request (comma-separated)")
	cmd.Flags().StringVar(&issuer, "issuer", "", "Authorization server issuer URL override")
	cmd.Flags().IntVar(&port, "port", 0, "TCP port for the localhost OAuth callback listener")
	cmd.Flags().StringVar(&redirectPath, "redirect-path", "", "Path of the localhost OAuth callback URI")

	return cmd
}

func newLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <name>",
		Short: "Remove stored OAuth credentials for an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			hadCredential := mcpHasCredFunc(name)
			if err := mcpLogoutFunc(name); err != nil {
				return err
			}
			if hadCredential {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Removed stored credentials for MCP server %q.\n", name)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "No stored credentials for MCP server %q.\n", name)
			}
			return nil
		},
	}
}

// ensureOAuthConfig returns the existing OAuth config or a fresh default one.
func ensureOAuthConfig(existing *config.MCPOAuthConfig) *config.MCPOAuthConfig {
	if existing != nil {
		return existing
	}
	return &config.MCPOAuthConfig{}
}

// applyAuthFlags merges non-zero CLI flag values into the OAuth config.
func applyAuthFlags(
	cfg *config.MCPOAuthConfig,
	clientID, clientSecret string,
	scopes []string,
	issuer string,
	port int,
	redirectPath string,
	noBrowser bool,
) {
	if cfg == nil {
		return
	}
	if clientID != "" {
		cfg.ClientID = clientID
	}
	if clientSecret != "" {
		cfg.ClientSecret = clientSecret
	}
	if len(scopes) > 0 {
		cfg.Scopes = scopes
	}
	if issuer != "" {
		cfg.Issuer = issuer
	}
	if port > 0 {
		cfg.CallbackPort = port
	}
	if redirectPath != "" {
		cfg.RedirectPath = redirectPath
	}
	if noBrowser {
		cfg.NoBrowser = true
	}
}