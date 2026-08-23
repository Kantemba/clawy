// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

package tools

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Kantemba/clawy/pkg/config"
)

// localShellNames lists the base names the local backend can resolve to.
var localShellNames = map[string]bool{
	"powershell":     true,
	"powershell.exe": true,
	"pwsh":           true,
	"pwsh.exe":       true,
	"sh":             true,
}

func TestEffectiveBackend(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"", config.TerminalBackendLocal},
		{"local", config.TerminalBackendLocal},
		{"  LOCAL ", config.TerminalBackendLocal},
		{"docker", config.TerminalBackendDocker},
		{"Docker", config.TerminalBackendDocker},
		{"ssh", config.TerminalBackendSSH},
		{"kubernetes", config.TerminalBackendLocal}, // unknown → safe fallback
	}
	for _, tt := range tests {
		if got := effectiveBackend(tt.raw); got != tt.want {
			t.Errorf("effectiveBackend(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestBuildTerminalCommand_Local(t *testing.T) {
	cmd := buildTerminalCommand(context.Background(), &config.ExecConfig{}, "echo hi", "")
	var want []string
	if runtime.GOOS == "windows" {
		want = []string{"-NoProfile", "-NonInteractive", "-Command", "echo hi"}
	} else {
		want = []string{"-c", "echo hi"}
	}
	if !reflect.DeepEqual(cmd.Args[1:], want) {
		t.Errorf("local args = %v, want %v", cmd.Args[1:], want)
	}
}

func TestBuildTerminalCommand_Docker(t *testing.T) {
	execCfg := &config.ExecConfig{
		Backend:         config.TerminalBackendDocker,
		DockerContainer: "dev-box",
		DockerShell:     "bash",
	}
	cmd := buildTerminalCommand(context.Background(), execCfg, "ls -la", "/workspace")
	want := []string{"exec", "-w", "/workspace", "dev-box", "bash", "-c", "ls -la"}
	if !reflect.DeepEqual(cmd.Args[1:], want) {
		t.Errorf("docker args = %v, want %v", cmd.Args[1:], want)
	}
}

func TestBuildTerminalCommand_DockerNoWorkdirOmitsFlag(t *testing.T) {
	execCfg := &config.ExecConfig{
		Backend:         config.TerminalBackendDocker,
		DockerContainer: "dev-box",
	}
	cmd := buildTerminalCommand(context.Background(), execCfg, "pwd", "   ")
	want := []string{"exec", "dev-box", "sh", "-c", "pwd"}
	if !reflect.DeepEqual(cmd.Args[1:], want) {
		t.Errorf("docker args = %v, want %v", cmd.Args[1:], want)
	}
}

func TestBuildTerminalCommand_DockerWithoutContainerFallsBackToLocal(t *testing.T) {
	execCfg := &config.ExecConfig{Backend: config.TerminalBackendDocker}
	cmd := buildTerminalCommand(context.Background(), execCfg, "echo hi", "")
	if got := strings.ToLower(filepath.Base(cmd.Path)); !localShellNames[got] {
		t.Errorf("expected local shell fallback, got %q", cmd.Path)
	}
}

func TestBuildTerminalCommand_SSH(t *testing.T) {
	execCfg := &config.ExecConfig{
		Backend:    config.TerminalBackendSSH,
		SSHHost:    "example.com",
		SSHPort:    2222,
		SSHUser:    "deploy",
		SSHKeyPath: "/keys/id_ed25519",
		SSHShell:   "bash",
	}
	cmd := buildTerminalCommand(context.Background(), execCfg, "systemctl status foo", "/srv/app")
	want := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-p", "2222",
		"-i", "/keys/id_ed25519",
		"deploy@example.com",
		"--", "bash", "-c", "cd '/srv/app' && systemctl status foo",
	}
	if !reflect.DeepEqual(cmd.Args[1:], want) {
		t.Errorf("ssh args = %v, want %v", cmd.Args[1:], want)
	}
}

func TestBuildTerminalCommand_SSHDefaults(t *testing.T) {
	execCfg := &config.ExecConfig{
		Backend: config.TerminalBackendSSH,
		SSHHost: "example.com",
	}
	cmd := buildTerminalCommand(context.Background(), execCfg, "uptime", "")
	want := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"example.com",
		"--", "sh", "-c", "uptime",
	}
	if !reflect.DeepEqual(cmd.Args[1:], want) {
		t.Errorf("ssh default args = %v, want %v (port 22 must be omitted)", cmd.Args[1:], want)
	}
}

func TestBuildTerminalCommand_SSHQuotesCwdWithSingleQuotes(t *testing.T) {
	execCfg := &config.ExecConfig{
		Backend: config.TerminalBackendSSH,
		SSHHost: "example.com",
	}
	cmd := buildTerminalCommand(context.Background(), execCfg, "ls", "/tmp/it's here")
	remoteCmd := cmd.Args[len(cmd.Args)-1]
	wantRemote := "cd '/tmp/it'\\''s here' && ls"
	if remoteCmd != wantRemote {
		t.Errorf("quoted remote command = %q, want %q", remoteCmd, wantRemote)
	}
}

func TestBuildTerminalCommand_SSHWithoutHostFallsBackToLocal(t *testing.T) {
	execCfg := &config.ExecConfig{Backend: config.TerminalBackendSSH}
	cmd := buildTerminalCommand(context.Background(), execCfg, "echo hi", "")
	if got := strings.ToLower(filepath.Base(cmd.Path)); !localShellNames[got] {
		t.Errorf("expected local shell fallback, got %q", cmd.Path)
	}
}

func TestPosixShellQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/srv/app", "'/srv/app'"},
		{"", "''"},
		{"it's", "'it'\\''s'"},
	}
	for _, tt := range tests {
		if got := posixShellQuote(tt.in); got != tt.want {
			t.Errorf("posixShellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBackendSupportsPTY(t *testing.T) {
	if !backendSupportsPTY(nil) {
		t.Error("nil config should default to local backend with PTY support")
	}
	if !backendSupportsPTY(&config.ExecConfig{}) {
		t.Error("empty config should have PTY support")
	}
	for _, backend := range []string{config.TerminalBackendDocker, config.TerminalBackendSSH} {
		if backendSupportsPTY(&config.ExecConfig{Backend: backend}) {
			t.Errorf("backend %q must not advertise PTY support", backend)
		}
	}
}

func TestDescribeBackend(t *testing.T) {
	if got := describeBackend(&config.ExecConfig{}); got != "" {
		t.Errorf("describeBackend(local) = %q, want empty", got)
	}
	docker := &config.ExecConfig{Backend: config.TerminalBackendDocker, DockerContainer: "dev-box"}
	if got := describeBackend(docker); got != "docker:dev-box" {
		t.Errorf("describeBackend(docker) = %q, want docker:dev-box", got)
	}
	ssh := &config.ExecConfig{Backend: config.TerminalBackendSSH, SSHHost: "example.com", SSHUser: "deploy"}
	if got := describeBackend(ssh); got != "ssh:deploy@example.com" {
		t.Errorf("describeBackend(ssh) = %q, want ssh:deploy@example.com", got)
	}
}