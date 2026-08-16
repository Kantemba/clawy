package agent

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/isolation"
	"github.com/Kantemba/clawy/pkg/tools"
)

// NewStandaloneToolRegistry builds a ToolRegistry from config without
// requiring an LLM provider. It registers the built-in file-system and exec
// tools that are enabled in the config.
//
// This is used by the `clawy mcp serve` CLI command to expose Clawy's
// built-in tools as a native MCP server.
func NewStandaloneToolRegistry(cfg *config.Config) *tools.ToolRegistry {
	registry := tools.NewToolRegistry()

	if cfg != nil {
		isolation.Configure(cfg)
	}

	workspace := cfg.WorkspacePath()
	if workspace == "" {
		workspace = os.TempDir()
	}
	os.MkdirAll(workspace, 0o755)

	allowReadPaths := buildAllowReadPatterns(cfg)
	restrict := true
	readRestrict := restrict && !cfg.Agents.Defaults.AllowReadOutsideWorkspace
	allowWritePaths := compilePatterns(cfg.Tools.AllowWritePaths)

	maxReadFileSize := cfg.Tools.ReadFile.MaxReadFileSize

	// read_file
	if cfg.Tools.IsToolEnabled("read_file") {
		switch cfg.Tools.ReadFile.EffectiveMode() {
		case config.ReadFileModeLines:
			registry.Register(tools.NewReadFileLinesTool(
				workspace, readRestrict, maxReadFileSize, allowReadPaths))
		default:
			registry.Register(tools.NewReadFileBytesTool(
				workspace, readRestrict, maxReadFileSize, allowReadPaths))
		}
	}

	// edit_file
	if cfg.Tools.IsToolEnabled("edit_file") {
		registry.Register(tools.NewEditFileTool(workspace, restrict, allowWritePaths))
	}

	// append_file
	if cfg.Tools.IsToolEnabled("append_file") {
		registry.Register(tools.NewAppendFileTool(workspace, restrict, allowWritePaths))
	}

	// write_file
	if cfg.Tools.IsToolEnabled("write_file") {
		writeTool := tools.NewWriteFileTool(workspace, restrict, allowWritePaths)
		var altTools []string
		if registry.HasRegistered("append_file") {
			altTools = append(altTools, "append_file")
		}
		if registry.HasRegistered("edit_file") {
			altTools = append(altTools, "edit_file")
		}
		writeTool.SetAlternativeTools(altTools)
		registry.Register(writeTool)
	}

	// list_dir
	if cfg.Tools.IsToolEnabled("list_dir") {
		registry.Register(tools.NewListDirTool(workspace, readRestrict, allowReadPaths))
	}

	// exec
	if cfg.Tools.IsToolEnabled("exec") {
		execTool, err := tools.NewExecToolWithConfig(workspace, restrict, cfg, allowReadPaths)
		if err != nil {
			// log but continue
		} else {
			registry.Register(execTool)
		}
	}

	return registry
}

// compilePatterns is already defined in instance.go, keep this comment for reference.
var _ = regexp.MustCompile
var _ = filepath.Join
