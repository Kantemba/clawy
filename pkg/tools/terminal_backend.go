// Clawy - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

package tools

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/logger"
)

// Terminal backends (Hermes-agent-style): the exec tool can run commands not
// only on the local host but also inside a Docker container or on a remote
// host over SSH. The backend is selected in config under tools.exec.backend;
// deny/allow patterns are evaluated on the command text before dispatch, so
// the same safety policy applies regardless of where the command runs.

// effectiveBackend normalizes and validates the configured backend. An empty
// value means "local". Unknown values fall back to local with a warning so a
// typo cannot silently break exec.
func effectiveBackend(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(config.TerminalBackendLocal):
		return config.TerminalBackendLocal
	case string(config.TerminalBackendDocker):
		return config.TerminalBackendDocker
	case string(config.TerminalBackendSSH):
		return config.TerminalBackendSSH
	default:
		logger.WarnCF("tools", "unknown exec backend, falling back to local", map[string]any{
			"backend": raw,
		})
		return config.TerminalBackendLocal
	}
}

// remoteShellName picks the shell used to wrap commands on non-local backends.
func remoteShellName(configured string) string {
	shell := strings.TrimSpace(configured)
	if shell == "" {
		shell = "sh"
	}
	return shell
}

// execCfgOrDefault returns cfg, or an empty ExecConfig when cfg is nil so
// zero-value constructors (e.g. NewExecTool without config) keep using the
// default local backend.
func execCfgOrDefault(cfg *config.ExecConfig) *config.ExecConfig {
	if cfg == nil {
		return &config.ExecConfig{}
	}
	return cfg
}

// buildTerminalCommand constructs the exec.Cmd for the configured backend.
//
//   - local:  powershell -Command <cmd> | sh -c <cmd>          (unchanged behavior)
//   - docker: docker exec [-w <cwd>] <container> <shell> -c <cmd>
//   - ssh:    ssh [-i key] [-p port] -o BatchMode=yes ... user@host -- <shell> -c 'cd <cwd> && <cmd>'
//
// cwd handling is backend-aware: local uses cmd.Dir, docker uses -w, and ssh
// prepends a quoted cd so relative paths resolve in the requested directory.
func buildTerminalCommand(ctx context.Context, execCfg *config.ExecConfig, command, cwd string) *exec.Cmd {
	execCfg = execCfgOrDefault(execCfg)
	backend := effectiveBackend(execCfg.Backend)

	switch backend {
	case config.TerminalBackendDocker:
		container := strings.TrimSpace(execCfg.DockerContainer)
		if container == "" {
			logger.WarnCF("tools", "docker backend selected without docker_container, using local shell", nil)
			break // fall through to local
		}
		args := []string{"exec"}
		if cwd = strings.TrimSpace(cwd); cwd != "" {
			args = append(args, "-w", cwd)
		}
		args = append(args, container, remoteShellName(execCfg.DockerShell), "-c", command)
		return exec.CommandContext(ctx, "docker", args...)

	case config.TerminalBackendSSH:
		host := strings.TrimSpace(execCfg.SSHHost)
		if host == "" {
			logger.WarnCF("tools", "ssh backend selected without ssh_host, using local shell", nil)
			break // fall through to local
		}
		target := host
		if user := strings.TrimSpace(execCfg.SSHUser); user != "" {
			target = user + "@" + host
		}
		args := []string{
			"-o", "BatchMode=yes",
			"-o", "StrictHostKeyChecking=accept-new",
		}
		if port := execCfg.SSHPort; port > 0 && port != 22 {
			args = append(args, "-p", fmt.Sprintf("%d", port))
		}
		if keyPath := strings.TrimSpace(execCfg.SSHKeyPath); keyPath != "" {
			args = append(args, "-i", keyPath)
		}
		remoteCmd := command
		if cwd = strings.TrimSpace(cwd); cwd != "" {
			remoteCmd = "cd " + posixShellQuote(cwd) + " && " + command
		}
		args = append(args, target, "--", remoteShellName(execCfg.SSHShell), "-c", remoteCmd)
		return exec.CommandContext(ctx, "ssh", args...)
	}

	// Local backend: same behavior as before backends existed.
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	if cwd = strings.TrimSpace(cwd); cwd != "" {
		cmd.Dir = cwd
	}
	return cmd
}

// posixShellQuote single-quotes s for safe interpolation into a POSIX shell
// command line, escaping embedded single quotes the standard way.
func posixShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// backendSupportsPTY reports whether interactive PTY background sessions can
// work on the configured backend. PTY allocation is a host-side pty.Open() on
// the spawned process, which is only meaningful for local processes — remote
// backends multiplex everything through the docker/ssh client pipe.
func backendSupportsPTY(execCfg *config.ExecConfig) bool {
	return effectiveBackend(execCfgOrDefault(execCfg).Backend) == config.TerminalBackendLocal
}

// describeBackend returns a short human label appended to exec results so the
// model knows where its command ran.
func describeBackend(execCfg *config.ExecConfig) string {
	execCfg = execCfgOrDefault(execCfg)
	backend := effectiveBackend(execCfg.Backend)
	switch backend {
	case config.TerminalBackendDocker:
		return "docker:" + strings.TrimSpace(execCfg.DockerContainer)
	case config.TerminalBackendSSH:
		target := strings.TrimSpace(execCfg.SSHHost)
		if user := strings.TrimSpace(execCfg.SSHUser); user != "" && target != "" {
			target = user + "@" + target
		}
		return "ssh:" + target
	default:
		return ""
	}
}