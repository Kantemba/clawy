package api

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kantemba/clawy/clients/web/backend/launcherconfig"
)

func TestResolveLaunchCommandUsesConfigFileDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	// Persist non-default launcher options to ensure resolveLaunchCommand does not
	// pin them into autostart args.
	launcherPath := launcherconfig.PathForAppConfig(configPath)
	if err := launcherconfig.Save(launcherPath, launcherconfig.Config{
		Port:   19999,
		Public: true,
	}); err != nil {
		t.Fatalf("launcherconfig.Save() error = %v", err)
	}

	exePath, args, err := h.resolveLaunchCommand()
	if err != nil {
		t.Fatalf("resolveLaunchCommand() error = %v", err)
	}
	if exePath == "" {
		t.Fatal("resolveLaunchCommand() returned empty executable path")
	}
	if len(args) != 3 || args[0] != "start" || args[1] != "--no-browser" || args[2] != configPath {
		t.Fatalf("args = %v, want [start --no-browser %s]", args, configPath)
	}
	for _, arg := range args {
		if arg == "-port" || arg == "-public" {
			t.Fatalf("autostart args should not pin network flags, got %v", args)
		}
	}
}

func TestResolveLaunchCommandIncludesDebugFlagWhenEnabled(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)
	h.SetDebug(true)

	_, args, err := h.resolveLaunchCommand()
	if err != nil {
		t.Fatalf("resolveLaunchCommand() error = %v", err)
	}
	if len(args) != 4 || args[0] != "start" || args[1] != "--no-browser" || args[2] != "--debug" || args[3] != configPath {
		t.Fatalf("args = %v, want [start --no-browser --debug %s]", args, configPath)
	}
}

func TestBuildDarwinPlistIncludesRunAtLoad(t *testing.T) {
	plist := buildDarwinPlist("/tmp/clawy-web", []string{"-no-browser", "/tmp/config.json"})
	if !strings.Contains(plist, "<key>RunAtLoad</key>") {
		t.Fatalf("plist missing RunAtLoad key:\n%s", plist)
	}
	if !strings.Contains(plist, "<true/>") {
		t.Fatalf("plist missing RunAtLoad true value:\n%s", plist)
	}
}
