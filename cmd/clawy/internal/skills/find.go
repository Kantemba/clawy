package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kantemba/clawy/cmd/clawy/internal"
	"github.com/Kantemba/clawy/pkg/skills"
	"github.com/Kantemba/clawy/pkg/utils"
)

func newFindCommand() *cobra.Command {
	var (
		registry string
		version  string
		force    bool
		limit    int
	)

	cmd := &cobra.Command{
		Use:   "find [query]",
		Short: "Discover and install a skill from registries",
		Long: `Search for a skill across all configured registries and install the best match.
		
This command combines search and install into one step — it queries all configured
registries (GitHub, ClawHub, etc.), picks the highest-scoring match, and installs
it to your workspace automatically.

Examples:
  clawy skills find github
  clawy skills find weather
  clawy skills find --registry clawhub docker
  clawy skills find --limit 3 database
`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			query := strings.TrimSpace(args[0])
			if query == "" {
				return fmt.Errorf("query is required")
			}

			cfg, err := internal.LoadConfig()
			if err != nil {
				return err
			}

			registryMgr := skills.NewRegistryManagerFromToolsConfig(cfg.Tools.Skills)

			var targetRegistry skills.SkillRegistry
			if registry != "" {
				if err := utils.ValidateSkillIdentifier(registry); err != nil {
					return fmt.Errorf("invalid registry: %w", err)
				}
				targetRegistry = registryMgr.GetRegistry(registry)
				if targetRegistry == nil {
					return fmt.Errorf("registry %q not found", registry)
				}
			}

			fmt.Printf("Searching for %q", query)
			if targetRegistry != nil {
				fmt.Printf(" in %s registry", targetRegistry.Name())
			}
			fmt.Println("...")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			var results []skills.SearchResult
			if targetRegistry != nil {
				results, err = targetRegistry.Search(ctx, query, limit)
			} else {
				results, err = registryMgr.SearchAll(ctx, query, limit)
			}
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}

			if len(results) == 0 {
				fmt.Println("No skills found.")
				return nil
			}

			best := results[0]
			fmt.Printf("\nBest match: %s (score: %.3f, registry: %s)\n", best.Slug, best.Score, best.RegistryName)
			if best.DisplayName != "" && best.DisplayName != best.Slug {
				fmt.Printf("  Name: %s\n", best.DisplayName)
			}
			if best.Summary != "" {
				fmt.Printf("  Description: %s\n", best.Summary)
			}

			resolvedRegistry := targetRegistry
			if resolvedRegistry == nil {
				resolvedRegistry = registryMgr.GetRegistry(best.RegistryName)
			}
			if resolvedRegistry == nil {
				return fmt.Errorf("registry %q no longer available", best.RegistryName)
			}

			dirName, err := resolvedRegistry.ResolveInstallDirName(best.Slug)
			if err != nil {
				return fmt.Errorf("invalid slug: %w", err)
			}

			workspace := cfg.WorkspacePath()
			targetDir := filepath.Join(workspace, "skills", dirName)

			if !force {
				if _, statErr := os.Stat(targetDir); statErr == nil {
					fmt.Printf("\nSkill already installed at %s\nUse --force to reinstall.\n", targetDir)
					return nil
				}
			}

			if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
				return fmt.Errorf("failed to create skills directory: %w", err)
			}

			installVersion := version
			if installVersion == "" {
				installVersion = best.Version
			}

			fmt.Printf("\nInstalling %s...\n", best.Slug)
			installResult, err := resolvedRegistry.DownloadAndInstall(ctx, best.Slug, installVersion, targetDir)
			if err != nil {
				_ = os.RemoveAll(targetDir)
				return fmt.Errorf("installation failed: %w", err)
			}

			if installResult.IsMalwareBlocked {
				_ = os.RemoveAll(targetDir)
				return fmt.Errorf("skill is flagged as malicious and cannot be installed")
			}

			if installResult.IsSuspicious {
				fmt.Println("⚠️  Warning: skill is flagged as suspicious.")
			}

			fmt.Printf("✓ Skill '%s' v%s installed to %s\n", dirName, installResult.Version, targetDir)
			warnSkillPermissions(targetDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&registry, "registry", "", "Search in a specific registry (e.g., github, clawhub)")
	cmd.Flags().StringVar(&version, "version", "", "Specific version to install (default: latest)")
	cmd.Flags().BoolVar(&force, "force", false, "Force reinstall if already exists")
	cmd.Flags().IntVar(&limit, "limit", 5, "Maximum results to consider (1-10)")

	return cmd
}
