package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPluginCommand(t *testing.T) {
	cmd := NewPluginCommand()

	assert.Equal(t, "plugin", cmd.Name())
	assert.NotNil(t, cmd.RunE)

	subcommands := cmd.Commands()
	allowed := []string{"info", "install", "list", "new"}
	require.Len(t, subcommands, len(allowed))

	for _, sub := range subcommands {
		found := false
		for _, name := range allowed {
			if sub.Name() == name {
				found = true
				break
			}
		}
		assert.True(t, found, "unexpected subcommand %q", sub.Name())
	}
}
