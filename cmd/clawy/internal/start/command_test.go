package start

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	webconsole "github.com/Kantemba/clawy/clients/web/backend"
)

func TestOptionsPreserveStoredDefaults(t *testing.T) {
	opts := optionsFor(NewStartCommand(), nil)
	require.Equal(t, "18800", opts.Port)
	require.False(t, opts.PortSet)
	require.False(t, opts.HostSet)
	require.False(t, opts.PublicSet)
	require.False(t, opts.NoBrowser)
	require.True(t, opts.Console)
}

func TestOptionsForwardExplicitSettings(t *testing.T) {
	cmd := NewStartCommand()
	for name, value := range map[string]string{
		"port": "19999", "host": "127.0.0.1", "public": "false", "no-browser": "true", "debug": "true",
	} {
		require.NoError(t, cmd.Flags().Set(name, value))
	}
	opts := optionsFor(cmd, []string{"a config.json"})
	require.Equal(t, "19999", opts.Port)
	require.Equal(t, "127.0.0.1", opts.Host)
	require.True(t, opts.PortSet)
	require.True(t, opts.HostSet)
	require.True(t, opts.PublicSet)
	require.False(t, opts.Public)
	require.True(t, opts.NoBrowser)
	require.True(t, opts.Debug)
	require.Equal(t, "a config.json", opts.ConfigPath)
}

func TestStartAcceptsOnlyOneConfigPath(t *testing.T) {
	cmd := NewStartCommand()
	require.NoError(t, cmd.Args(cmd, nil))
	require.NoError(t, cmd.Args(cmd, []string{"config.json"}))
	require.Error(t, cmd.Args(cmd, []string{"one", "two"}))
}

func TestStartRunsInProcessWithoutLauncher(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	original := runWebConsole
	t.Cleanup(func() { runWebConsole = original })
	wantErr := errors.New("console stopped")
	called := false
	runWebConsole = func(ctx context.Context, opts webconsole.Options) error {
		called = true
		require.NotNil(t, ctx)
		require.True(t, opts.NoBrowser)
		require.Equal(t, "custom.json", opts.ConfigPath)
		return wantErr
	}
	cmd := NewStartCommand()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--no-browser", "custom.json"})
	require.ErrorIs(t, cmd.Execute(), wantErr)
	require.True(t, called)
}
