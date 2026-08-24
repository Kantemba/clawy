package mcp

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/pkg/config"
)

type addOptions struct {
	Env       []string
	EnvFile   string
	Headers   []string
	Transport string
	Force     bool
	Deferred  *bool // nil = not set, true = deferred, false = not deferred

	oauthEnabled      *bool // nil = not set, true = --oauth, false = --no-oauth
	oauthClientID     string
	oauthClientSecret string
	oauthScopes       []string
	oauthIssuer       string
	oauthPort         int
	oauthRedirectPath string
	oauthNoBrowser    bool
	oauthSeen         bool // any --oauth* flag was provided
}

// buildOAuthConfig assembles the OAuth configuration from --oauth* flags.
// It returns nil when no OAuth flag was provided at all.
func (o *addOptions) buildOAuthConfig() *config.MCPOAuthConfig {
	if !o.oauthSeen {
		return nil
	}
	return &config.MCPOAuthConfig{
		Enabled:      o.oauthEnabled,
		ClientID:     o.oauthClientID,
		ClientSecret: o.oauthClientSecret,
		Scopes:       append([]string(nil), o.oauthScopes...),
		Issuer:       o.oauthIssuer,
		CallbackPort: o.oauthPort,
		RedirectPath: o.oauthRedirectPath,
		NoBrowser:    o.oauthNoBrowser,
	}
}

// nextFlagValue consumes the value following a space-separated flag.
func nextFlagValue(args []string, i *int, flag string) (string, error) {
	if *i+1 >= len(args) {
		return "", fmt.Errorf("missing value for %s", flag)
	}
	*i++
	return args[*i], nil
}

// appendOAuthScopes splits a raw scope list on commas and whitespace and
// appends the entries to dst.
func appendOAuthScopes(dst []string, raw string) []string {
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		dst = append(dst, part)
	}
	return dst
}

func newAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "add [flags] <name> <command-or-url> [args...]",
		Short:              "Add or update an MCP server",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, name, target, targetArgs, showHelp, err := parseAddArgs(args)
			if showHelp {
				return cmd.Help()
			}
			if err != nil {
				return err
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if cfg.Tools.MCP.Servers == nil {
				cfg.Tools.MCP.Servers = make(map[string]config.MCPServerConfig)
			}

			if _, exists := cfg.Tools.MCP.Servers[name]; exists && !opts.Force {
				var overwrite bool

				overwrite, err = confirmOverwrite(cmd.InOrStdin(), cmd.OutOrStdout(), name)
				if err != nil {
					return fmt.Errorf("failed to confirm overwrite: %w", err)
				}
				if !overwrite {
					return fmt.Errorf("aborted: MCP server %q already exists", name)
				}
			}

			server, err := buildServerConfig(target, targetArgs, opts)
			if err != nil {
				return err
			}

			cfg.Tools.MCP.Enabled = true
			cfg.Tools.MCP.Servers[name] = server

			if err := saveValidatedConfig(cfg); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✓ MCP server %q saved.\n", name)
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringArrayP("env", "e", nil, "Environment variable in KEY=value format (repeatable, saved to config)")
	flags.String("env-file", "", "Path to an env file for stdio servers (recommended for secrets)")
	flags.StringArrayP("header", "H", nil, "HTTP header in 'Name: Value' or 'Name=Value' format (repeatable)")
	flags.StringP("transport", "t", "stdio", "Transport type: stdio, http / streamable-http, or sse")
	flags.BoolP("force", "f", false, "Overwrite an existing server without prompting")
	flags.Bool("deferred", false, "Mark server as deferred (tools hidden until explicitly activated)")
	flags.Bool("no-deferred", false, "Mark server as non-deferred (tools always active)")
	flags.Bool("oauth", false, "Enable OAuth authentication (sse/http servers only)")
	flags.Bool("no-oauth", false, "Explicitly disable OAuth authentication")
	flags.String("oauth-client-id", "", "Pre-registered OAuth client id")
	flags.String("oauth-client-secret", "", "OAuth client secret for confidential clients")
	flags.String("oauth-scopes", "", "OAuth scopes to request (comma or space separated, repeatable)")
	flags.String("oauth-issuer", "", "Authorization server issuer URL override")
	flags.Int("oauth-port", 0, "TCP port for the localhost OAuth callback listener")
	flags.String("oauth-redirect-path", "", "Path of the localhost OAuth callback URI (default /callback)")
	flags.Bool("oauth-no-browser", false, "Do not open the browser automatically during OAuth")

	return cmd
}

func parseAddArgs(args []string) (addOptions, string, string, []string, bool, error) {
	opts := addOptions{Transport: "stdio"}
	var positional []string
	serverArgs := make([]string, 0)
	explicitCommand := make([]string, 0)

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--help" || arg == "-h":
			return addOptions{}, "", "", nil, true, nil
		case arg == "--":
			if i+1 < len(args) {
				explicitCommand = append(explicitCommand, args[i+1:]...)
			}
			i = len(args)
		case arg == "--force" || arg == "-f":
			opts.Force = true
		case arg == "--deferred":
			t := true
			opts.Deferred = &t
		case arg == "--no-deferred":
			f := false
			opts.Deferred = &f
		case arg == "--transport" || arg == "-t":
			if i+1 >= len(args) {
				return addOptions{}, "", "", nil, false, fmt.Errorf("missing value for %s", arg)
			}
			i++
			opts.Transport = args[i]
		case strings.HasPrefix(arg, "--transport="):
			opts.Transport = strings.TrimPrefix(arg, "--transport=")
		case arg == "--env" || arg == "-e":
			if i+1 >= len(args) {
				return addOptions{}, "", "", nil, false, fmt.Errorf("missing value for %s", arg)
			}
			i++
			opts.Env = append(opts.Env, args[i])
		case arg == "--env-file":
			if i+1 >= len(args) {
				return addOptions{}, "", "", nil, false, fmt.Errorf("missing value for %s", arg)
			}
			i++
			opts.EnvFile = args[i]
		case strings.HasPrefix(arg, "--env="):
			opts.Env = append(opts.Env, strings.TrimPrefix(arg, "--env="))
		case strings.HasPrefix(arg, "--env-file="):
			opts.EnvFile = strings.TrimPrefix(arg, "--env-file=")
		case arg == "--header" || arg == "-H":
			if i+1 >= len(args) {
				return addOptions{}, "", "", nil, false, fmt.Errorf("missing value for %s", arg)
			}
			i++
			opts.Headers = append(opts.Headers, args[i])
		case strings.HasPrefix(arg, "--header="):
			opts.Headers = append(opts.Headers, strings.TrimPrefix(arg, "--header="))
		case arg == "--oauth":
			t := true
			opts.oauthEnabled, opts.oauthSeen = &t, true
		case arg == "--no-oauth":
			f := false
			opts.oauthEnabled, opts.oauthSeen = &f, true
		case arg == "--oauth-client-id":
			v, verr := nextFlagValue(args, &i, arg)
			if verr != nil {
				return addOptions{}, "", "", nil, false, verr
			}
			opts.oauthClientID, opts.oauthSeen = v, true
		case strings.HasPrefix(arg, "--oauth-client-id="):
			opts.oauthClientID = strings.TrimPrefix(arg, "--oauth-client-id=")
			opts.oauthSeen = true
		case arg == "--oauth-client-secret":
			v, verr := nextFlagValue(args, &i, arg)
			if verr != nil {
				return addOptions{}, "", "", nil, false, verr
			}
			opts.oauthClientSecret, opts.oauthSeen = v, true
		case strings.HasPrefix(arg, "--oauth-client-secret="):
			opts.oauthClientSecret = strings.TrimPrefix(arg, "--oauth-client-secret=")
			opts.oauthSeen = true
		case arg == "--oauth-issuer":
			v, verr := nextFlagValue(args, &i, arg)
			if verr != nil {
				return addOptions{}, "", "", nil, false, verr
			}
			opts.oauthIssuer, opts.oauthSeen = v, true
		case strings.HasPrefix(arg, "--oauth-issuer="):
			opts.oauthIssuer = strings.TrimPrefix(arg, "--oauth-issuer=")
			opts.oauthSeen = true
		case arg == "--oauth-redirect-path":
			v, verr := nextFlagValue(args, &i, arg)
			if verr != nil {
				return addOptions{}, "", "", nil, false, verr
			}
			opts.oauthRedirectPath, opts.oauthSeen = v, true
		case strings.HasPrefix(arg, "--oauth-redirect-path="):
			opts.oauthRedirectPath = strings.TrimPrefix(arg, "--oauth-redirect-path=")
			opts.oauthSeen = true
		case arg == "--oauth-scopes":
			v, verr := nextFlagValue(args, &i, arg)
			if verr != nil {
				return addOptions{}, "", "", nil, false, verr
			}
			opts.oauthScopes = appendOAuthScopes(opts.oauthScopes, v)
			opts.oauthSeen = true
		case strings.HasPrefix(arg, "--oauth-scopes="):
			opts.oauthScopes = appendOAuthScopes(opts.oauthScopes, strings.TrimPrefix(arg, "--oauth-scopes="))
			opts.oauthSeen = true
		case arg == "--oauth-port":
			v, verr := nextFlagValue(args, &i, arg)
			if verr != nil {
				return addOptions{}, "", "", nil, false, verr
			}
			portVal, perr := strconv.Atoi(strings.TrimSpace(v))
			if perr != nil || portVal < 0 {
				return addOptions{}, "", "", nil, false, fmt.Errorf("invalid value for %s: %q", arg, v)
			}
			opts.oauthPort, opts.oauthSeen = portVal, true
		case strings.HasPrefix(arg, "--oauth-port="):
			rawPort := strings.TrimPrefix(arg, "--oauth-port=")
			portVal, perr := strconv.Atoi(strings.TrimSpace(rawPort))
			if perr != nil || portVal < 0 {
				return addOptions{}, "", "", nil, false, fmt.Errorf("invalid value for --oauth-port: %q", rawPort)
			}
			opts.oauthPort, opts.oauthSeen = portVal, true
		case arg == "--oauth-no-browser":
			opts.oauthNoBrowser, opts.oauthSeen = true, true
		case strings.HasPrefix(arg, "-") && len(positional) >= 2:
			serverArgs = append(serverArgs, args[i:]...)
			i = len(args)
		default:
			positional = append(positional, arg)
		}
	}

	if len(explicitCommand) > 0 {
		if len(positional) != 1 {
			return addOptions{}, "", "", nil, false, fmt.Errorf(
				"usage: clawy mcp add [flags] <name> <command-or-url> [args...] or clawy mcp add [flags] <name> -- <command> [args...]",
			)
		}
		if len(explicitCommand) == 0 {
			return addOptions{}, "", "", nil, false, fmt.Errorf("missing stdio command after --")
		}
		return opts, positional[0], explicitCommand[0], explicitCommand[1:], false, nil
	}

	if len(positional) < 2 {
		return addOptions{}, "", "", nil, false, fmt.Errorf(
			"usage: clawy mcp add [flags] <name> <command-or-url> [args...] or clawy mcp add [flags] <name> -- <command> [args...]",
		)
	}

	targetArgs := make([]string, 0, len(positional)-2+len(serverArgs))
	targetArgs = append(targetArgs, positional[2:]...)
	targetArgs = append(targetArgs, serverArgs...)

	return opts, positional[0], positional[1], targetArgs, false, nil
}

func buildServerConfig(target string, args []string, opts addOptions) (config.MCPServerConfig, error) {
	transport := config.NormalizeMCPTransportType(opts.Transport)
	if transport == "" {
		transport = "stdio"
	}
	switch transport {
	case "stdio", "http", "sse":
	default:
		return config.MCPServerConfig{}, fmt.Errorf("unsupported transport %q", opts.Transport)
	}

	env, err := parseEnvAssignments(opts.Env)
	if err != nil {
		return config.MCPServerConfig{}, err
	}
	headers, err := parseHeaderAssignments(opts.Headers)
	if err != nil {
		return config.MCPServerConfig{}, err
	}

	server := config.MCPServerConfig{
		Enabled:  true,
		Type:     transport,
		Deferred: opts.Deferred,
	}

	switch transport {
	case "http", "sse":
		if len(env) > 0 {
			return config.MCPServerConfig{}, fmt.Errorf("--env can only be used with stdio transport")
		}
		if strings.TrimSpace(opts.EnvFile) != "" {
			return config.MCPServerConfig{}, fmt.Errorf("--env-file can only be used with stdio transport")
		}
		if len(args) > 0 {
			return config.MCPServerConfig{}, fmt.Errorf("%s transport does not accept command arguments", transport)
		}
		parsedURL, err := url.ParseRequestURI(target)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return config.MCPServerConfig{}, fmt.Errorf("invalid MCP URL %q", target)
		}
		server.URL = target
		server.Headers = headers
		server.OAuth = opts.buildOAuthConfig()
		return server, nil
	}

	if len(headers) > 0 {
		return config.MCPServerConfig{}, fmt.Errorf("--header can only be used with http or sse transport")
	}

	if opts.oauthSeen {
		return config.MCPServerConfig{}, fmt.Errorf("--oauth options can only be used with http or sse transport")
	}

	if looksLikeRemoteURL(target) {
		return config.MCPServerConfig{}, fmt.Errorf(
			"target %q looks like a remote MCP URL, but transport is %q. Use --transport http or --transport sse",
			target,
			transport,
		)
	}

	command := target
	commandArgs := append([]string(nil), args...)

	if err := validateLocalCommandPath(target); err != nil {
		return config.MCPServerConfig{}, err
	}
	if isLocalCommandPath(command) {
		command = expandHomePath(command)
	}

	server.Command = command
	server.Args = commandArgs
	server.Env = env
	server.EnvFile = strings.TrimSpace(opts.EnvFile)

	return server, nil
}
