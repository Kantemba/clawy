package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Kantemba/clawy/pkg/config"
)

func TestMCPAddPersistsOAuthConfiguration(t *testing.T) {
	configPath := setupMCPConfigEnv(t)

	cmd := NewMCPCommand()
	output, err := executeCommand(cmd, []string{
		"add", "linear", "--transport", "http",
		"--oauth", "--oauth-client-id", "cid", "--oauth-client-secret", "secret",
		"--oauth-scopes", "read,write", "--oauth-port", "19877", "--oauth-no-browser",
		"https://mcp.linear.app/sse",
	}, "")
	require.NoError(t, err)
	assert.Contains(t, output, `MCP server "linear" saved`)

	cfg := readMCPConfig(t, configPath)
	srv := cfg.Tools.MCP.Servers["linear"]
	require.NotNil(t, srv.OAuth)
	assert.True(t, srv.OAuth.IsEnabled())
	assert.Equal(t, "cid", srv.OAuth.ClientID)
	assert.Equal(t, "secret", srv.OAuth.ClientSecret)
	assert.Equal(t, []string{"read", "write"}, srv.OAuth.Scopes)
	assert.Equal(t, 19877, srv.OAuth.CallbackPort)
	assert.True(t, srv.OAuth.NoBrowser)
	assert.Equal(t, "https://mcp.linear.app/sse", srv.URL)
}

func TestMCPAddNoOAuthFlagStoresDisabledBlock(t *testing.T) {
	configPath := setupMCPConfigEnv(t)

	cmd := NewMCPCommand()
	_, err := executeCommand(cmd, []string{
		"add", "svc", "--transport", "http", "--no-oauth", "https://mcp.example.com/mcp",
	}, "")
	require.NoError(t, err)

	cfg := readMCPConfig(t, configPath)
	srv := cfg.Tools.MCP.Servers["svc"]
	require.NotNil(t, srv.OAuth)
	assert.False(t, srv.OAuth.IsEnabled())
}

func TestMCPAddRejectsOAuthForStdio(t *testing.T) {
	setupMCPConfigEnv(t)

	cmd := NewMCPCommand()
	_, err := executeCommand(cmd, []string{"add", "x", "--oauth", "npx", "-y", "@foo/bar"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--oauth options can only be used with http or sse transport")
}

func TestMCPAuthCommandRunsLoginAndSavesConfig(t *testing.T) {
	configPath := setupMCPConfigEnv(t)
	base := config.DefaultConfig()
	base.Tools.MCP.Enabled = true
	base.Tools.MCP.Servers = map[string]config.MCPServerConfig{
		"remote": {Enabled: true, Type: "http", URL: "https://mcp.example.com/mcp"},
	}
	writeMCPConfig(t, configPath, base)

	var gotName string
	var gotCfg config.MCPServerConfig
	origLogin := mcpLoginFunc
	mcpLoginFunc = func(_ context.Context, name string, server config.MCPServerConfig) error {
		gotName = name
		gotCfg = server
		return nil
	}
	t.Cleanup(func() { mcpLoginFunc = origLogin })

	cmd := NewMCPCommand()
	out, err := executeCommand(cmd, []string{"auth", "remote", "--no-browser"}, "")
	require.NoError(t, err)
	assert.Equal(t, "remote", gotName)
	require.NotNil(t, gotCfg.OAuth)
	assert.True(t, gotCfg.OAuth.NoBrowser)
	assert.Contains(t, out, "authenticated")

	saved := readMCPConfig(t, configPath)
	require.NotNil(t, saved.Tools.MCP.Servers["remote"].OAuth)
	assert.True(t, saved.Tools.MCP.Servers["remote"].OAuth.IsEnabled())
}

func TestMCPAuthCommandRequiresRemoteServer(t *testing.T) {
	configPath := setupMCPConfigEnv(t)
	base := config.DefaultConfig()
	base.Tools.MCP.Servers = map[string]config.MCPServerConfig{
		"loc": {Enabled: true, Command: "foo"},
	}
	writeMCPConfig(t, configPath, base)

	cmd := NewMCPCommand()
	_, err := executeCommand(cmd, []string{"auth", "loc"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stdio transport")
}

func TestMCPAuthCommandLoginFailurePropagates(t *testing.T) {
	configPath := setupMCPConfigEnv(t)
	base := config.DefaultConfig()
	base.Tools.MCP.Servers = map[string]config.MCPServerConfig{
		"remote": {Enabled: true, Type: "http", URL: "https://mcp.example.com/mcp"},
	}
	writeMCPConfig(t, configPath, base)

	origLogin := mcpLoginFunc
	mcpLoginFunc = func(context.Context, string, config.MCPServerConfig) error {
		return errors.New("state mismatch")
	}
	t.Cleanup(func() { mcpLoginFunc = origLogin })

	cmd := NewMCPCommand()
	_, err := executeCommand(cmd, []string{"auth", "remote"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state mismatch")
}

func TestMCPLogoutCommandRemovesCredential(t *testing.T) {
	setupMCPConfigEnv(t)

	calls := 0
	origLogout, origHas := mcpLogoutFunc, mcpHasCredFunc
	mcpLogoutFunc = func(string) error { calls++; return nil }
	mcpHasCredFunc = func(string) bool { return true }
	t.Cleanup(func() { mcpLogoutFunc, mcpHasCredFunc = origLogout, origHas })

	cmd := NewMCPCommand()
	out, err := executeCommand(cmd, []string{"logout", "anything"}, "")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Contains(t, out, "Removed stored credentials")
}

func TestMCPLogoutCommandSurfacesError(t *testing.T) {
	setupMCPConfigEnv(t)

	origLogout := mcpLogoutFunc
	mcpLogoutFunc = func(string) error { return errors.New("boom") }
	t.Cleanup(func() { mcpLogoutFunc = origLogout })

	cmd := NewMCPCommand()
	_, err := executeCommand(cmd, []string{"logout", "x"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}