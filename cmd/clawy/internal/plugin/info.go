package plugin

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/pkg/plugins"
)

func newInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "info <name>",
		Short:   "Show what a plugin provides",
		Example: `clawy plugin info notion`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			bundle := plugins.Discover(cfg)
			plugin, ok := bundle.Find(args[0])
			if !ok {
				return fmt.Errorf("plugin %q not found (run `clawy plugin list`)", args[0])
			}

			out := cmd.OutOrStdout()
			manifest := plugin.Manifest
			fmt.Fprintf(out, "Name:        %s\n", plugin.Name())
			fmt.Fprintf(out, "Version:     %s\n", plugin.Version())
			fmt.Fprintf(out, "Format:      %s\n", plugin.Format())
			if description := plugin.Description(); description != "" {
				fmt.Fprintf(out, "Description: %s\n", description)
			}
			if author := manifest.Author.String(); author != "" {
				fmt.Fprintf(out, "Author:      %s\n", author)
			}
			if manifest.Homepage != "" {
				fmt.Fprintf(out, "Homepage:    %s\n", manifest.Homepage)
			}
			fmt.Fprintf(out, "Directory:   %s\n", plugin.Dir)
			fmt.Fprintf(out, "Manifest:    %s\n", manifest.ManifestPath)

			if len(plugin.SkillRoots) > 0 {
				fmt.Fprintln(out, "\nSkills:")
				for _, root := range plugin.SkillRoots {
					for _, name := range skillNamesIn(root) {
						fmt.Fprintf(out, "  %s  (%s)\n", name, filepath.Join(root, name, "SKILL.md"))
					}
				}
			}

			if len(plugin.Commands) > 0 {
				fmt.Fprintln(out, "\nCommands:")
				for _, def := range plugin.Commands {
					description := def.Description
					if description == "" {
						description = "prompt command"
					}
					fmt.Fprintf(out, "  /%s  %s\n", def.Name, description)
				}
			}

			if len(plugin.MCPServers) > 0 {
				fmt.Fprintln(out, "\nMCP servers:")
				for _, name := range sortedKeys(plugin.MCPServers) {
					server := plugin.MCPServers[name]
					target := server.URL
					if target == "" {
						target = strings.TrimSpace(strings.Join(append([]string{server.Command}, server.Args...), " "))
					}
					fmt.Fprintf(out, "  %s  %s\n", name, target)
				}
			}

			if len(plugin.Hooks.Processes) > 0 {
				fmt.Fprintln(out, "\nProcess hooks:")
				for name := range plugin.Hooks.Processes {
					fmt.Fprintf(out, "  %s\n", name)
				}
			}
			if len(plugin.Hooks.Builtins) > 0 {
				fmt.Fprintln(out, "\nBuiltin hooks:")
				for name := range plugin.Hooks.Builtins {
					fmt.Fprintf(out, "  %s\n", name)
				}
			}

			return nil
		},
	}
}

func skillNamesIn(root string) []string {
	entries, err := filepath.Glob(filepath.Join(root, "*", "SKILL.md"))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, filepath.Base(filepath.Dir(entry)))
	}
	return names
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
