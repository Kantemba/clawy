package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Kantemba/clawy/pkg/config"
)

func TestFindClawyBinaryUsesThisExecutable(t *testing.T) {
	t.Setenv(config.EnvBinary, "")
	t.Setenv("PATH", t.TempDir())
	exe, err := os.Executable()
	require.NoError(t, err)
	require.Equal(t, exe, FindClawyBinary(), "gateway must use the same installed binary, not a companion or PATH lookup")
}
