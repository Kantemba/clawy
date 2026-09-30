// Clawy - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Clawy contributors

package config

import (
	"os"
	"path/filepath"

	"github.com/Kantemba/clawy/pkg"
)

// Runtime environment variable keys for the clawy process.
// These control the location of files and binaries at runtime and are read
// directly via os.Getenv / os.LookupEnv. All clawy-specific keys use the
// CLAWY_ prefix. Reference these constants instead of inline string
// literals to keep all supported knobs visible in one place and to prevent
// typos.
const (
	// EnvHome overrides the base directory for all clawy data
	// (config, workspace, skills, auth store, …).
	// Default: ~/.clawy
	EnvHome = "CLAWY_HOME"

	// EnvConfig overrides the full path to the JSON config file.
	// Default: $CLAWY_HOME/config.json
	EnvConfig = "CLAWY_CONFIG"

	// EnvBuiltinSkills overrides the directory from which built-in
	// skills are loaded.
	// Default: <cwd>/skills
	EnvBuiltinSkills = "CLAWY_BUILTIN_SKILLS"

	// EnvBinary overrides the path to the clawy executable.
	// Used by the web launcher when spawning the gateway subprocess.
	// Default: resolved from the same directory as the current executable.
	EnvBinary = "CLAWY_BINARY"

	// EnvGatewayHost overrides the host address for the gateway server.
	// Default: "localhost"
	EnvGatewayHost = "CLAWY_GATEWAY_HOST"

	// EnvPlugins lists extra plugin search directories (os.PathListSeparator
	// separated, e.g. "/opt/plugins:/home/me/plugins"). These are scanned in
	// addition to the config `plugins.dirs` entries and the built-in
	// <workspace>/plugins and ~/.clawy/plugins roots.
	EnvPlugins = "CLAWY_PLUGINS"
)

func GetHome() string {
	homePath, _ := os.UserHomeDir()
	if clawyHome := os.Getenv(EnvHome); clawyHome != "" {
		homePath = clawyHome
	} else if homePath != "" {
		homePath = filepath.Join(homePath, pkg.DefaultClawyHome)
	}
	if homePath == "" {
		homePath = "."
	}
	return homePath
}
