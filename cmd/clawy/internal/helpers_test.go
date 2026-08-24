package internal

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Kantemba/clawy/pkg/config"
)

func TestGetConfigPath(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")
	// os.UserHomeDir reads USERPROFILE on Windows.
	t.Setenv("USERPROFILE", "/tmp/home")

	got := GetConfigPath()
	want := filepath.Join("/tmp/home", ".clawy", "config.json")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithCLAWY_HOME(t *testing.T) {
	t.Setenv(config.EnvHome, "/custom/clawy")
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	want := filepath.Join("/custom/clawy", "config.json")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithCLAWY_CONFIG(t *testing.T) {
	t.Setenv("CLAWY_CONFIG", "/custom/config.json")
	t.Setenv(config.EnvHome, "/custom/clawy")
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	want := "/custom/config.json"

	assert.Equal(t, want, got)
}

func TestGetConfigPath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific HOME behavior varies; run on windows")
	}

	testUserProfilePath := `C:\Users\Test`
	t.Setenv("USERPROFILE", testUserProfilePath)

	got := GetConfigPath()
	want := filepath.Join(testUserProfilePath, ".clawy", "config.json")

	require.True(t, strings.EqualFold(got, want), "GetConfigPath() = %q, want %q", got, want)
}
