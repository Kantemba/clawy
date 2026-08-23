package integrationtools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Kantemba/clawy/pkg/logger"
	"github.com/Kantemba/clawy/pkg/skills"
	"github.com/Kantemba/clawy/pkg/utils"
)

// FindSkillTool discovers and installs skills from registries in one step.
// It searches across all configured registries, picks the best match,
// and installs it to the workspace automatically.
type FindSkillTool struct {
	registryMgr *skills.RegistryManager
	workspace   string
	cache       *skills.SearchCache
	mu          sync.Mutex
}

// NewFindSkillTool creates a new FindSkillTool.
func NewFindSkillTool(registryMgr *skills.RegistryManager, workspace string, cache *skills.SearchCache) *FindSkillTool {
	return &FindSkillTool{
		registryMgr: registryMgr,
		workspace:   workspace,
		cache:       cache,
		mu:          sync.Mutex{},
	}
}

func (t *FindSkillTool) Name() string {
	return "find_skill"
}

func (t *FindSkillTool) Description() string {
	return "Discover and install a skill from skill registries in one step. Searches across all configured registries (GitHub, ClawHub, etc.), finds the best matching skill, and installs it to the workspace. Returns the installed skill details on success."
}

func (t *FindSkillTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query describing the desired skill capability (e.g., 'github', 'weather', 'database management')",
			},
			"registry": map[string]any{
				"type":        "string",
				"description": "Registry to search in (optional, searches all configured registries if omitted)",
			},
			"version": map[string]any{
				"type":        "string",
				"description": "Specific version to install (optional, defaults to latest)",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "Force reinstall if skill already exists (default false)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to consider (1-10, default 5)",
				"minimum":     1.0,
				"maximum":     10.0,
			},
		},
		"required": []string{"query"},
	}
}

func (t *FindSkillTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return ErrorResult("query is required and must be a non-empty string")
	}
	query = strings.ToLower(strings.TrimSpace(query))

	registryName, _ := args["registry"].(string)
	registryName = strings.TrimSpace(registryName)
	version, _ := args["version"].(string)
	force, _ := args["force"].(bool)

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		li := int(l)
		if li >= 1 && li <= 10 {
			limit = li
		}
	}

	var results []skills.SearchResult
	var searchErr error

	if registryName != "" {
		if err := utils.ValidateSkillIdentifier(registryName); err != nil {
			return ErrorResult(fmt.Sprintf("invalid registry %q: %s", registryName, err.Error()))
		}
		registry := t.registryMgr.GetRegistry(registryName)
		if registry == nil {
			return ErrorResult(fmt.Sprintf("registry %q not found", registryName))
		}
		results, searchErr = t.searchRegistry(ctx, registry, query, limit)
	} else {
		results, searchErr = t.searchAllRegistries(ctx, query, limit)
	}

	if searchErr != nil {
		return ErrorResult(fmt.Sprintf("search failed: %v", searchErr))
	}
	if len(results) == 0 {
		return SilentResult(fmt.Sprintf("No skills found matching %q in any registry.", query))
	}

	best := results[0]
	return t.installBestMatch(ctx, best, version, force)
}

func (t *FindSkillTool) searchRegistry(ctx context.Context, registry skills.SkillRegistry, query string, limit int) ([]skills.SearchResult, error) {
	if t.cache != nil {
		cacheKey := registry.Name() + ":" + query
		if cached, hit := t.cache.Get(cacheKey); hit {
			return cached, nil
		}
		results, err := registry.Search(ctx, query, limit)
		if err == nil && len(results) > 0 {
			t.cache.Put(cacheKey, results)
		}
		return results, err
	}
	return registry.Search(ctx, query, limit)
}

func (t *FindSkillTool) searchAllRegistries(ctx context.Context, query string, limit int) ([]skills.SearchResult, error) {
	if t.cache != nil {
		if cached, hit := t.cache.Get("all:" + query); hit {
			return cached, nil
		}
	}
	results, err := t.registryMgr.SearchAll(ctx, query, limit)
	if err == nil && t.cache != nil && len(results) > 0 {
		t.cache.Put("all:"+query, results)
	}
	return results, err
}

func (t *FindSkillTool) installBestMatch(ctx context.Context, result skills.SearchResult, version string, force bool) *ToolResult {
	registry := t.registryMgr.GetRegistry(result.RegistryName)
	if registry == nil {
		return ErrorResult(fmt.Sprintf("registry %q no longer available", result.RegistryName))
	}

	dirName, err := registry.ResolveInstallDirName(result.Slug)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid slug %q: %s", result.Slug, err.Error()))
	}

	installVersion := version
	if installVersion == "" {
		installVersion = result.Version
	}

	skillsDir := filepath.Join(t.workspace, "skills")
	targetDir := filepath.Join(skillsDir, dirName)

	if !force {
		if _, statErr := os.Stat(targetDir); statErr == nil {
			return SilentResult(fmt.Sprintf(
				"Skill %q already installed at %s. Use force=true to reinstall.\nFound: %s (%s) from %s registry.",
				dirName, targetDir, result.DisplayName, result.Summary, result.RegistryName,
			))
		}
	}

	if mkdirErr := os.MkdirAll(skillsDir, 0o755); mkdirErr != nil {
		return ErrorResult(fmt.Sprintf("failed to create skills directory: %v", mkdirErr))
	}

	backupDir := ""
	restorePrevious := func() {
		if backupDir == "" {
			return
		}
		_ = os.RemoveAll(targetDir)
		_ = os.Rename(backupDir, targetDir)
		backupDir = ""
	}

	if force {
		if _, statErr := os.Stat(targetDir); statErr == nil {
			backupDir = filepath.Join(skillsDir, fmt.Sprintf(".%s.find-backup-%d", dirName, time.Now().UnixNano()))
			if renameErr := os.Rename(targetDir, backupDir); renameErr != nil {
				return ErrorResult(fmt.Sprintf("failed to prepare reinstall for %q: %v", result.Slug, renameErr))
			}
		}
	}

	installResult, err := registry.DownloadAndInstall(ctx, result.Slug, installVersion, targetDir)
	if err != nil {
		_ = os.RemoveAll(targetDir)
		restorePrevious()
		return ErrorResult(fmt.Sprintf("failed to install %q: %v", result.Slug, err))
	}

	if installResult.IsMalwareBlocked {
		_ = os.RemoveAll(targetDir)
		restorePrevious()
		return ErrorResult(fmt.Sprintf("skill %q is flagged as malicious and cannot be installed", result.Slug))
	}

	if !skills.SkillDirIsValid(targetDir) {
		_ = os.RemoveAll(targetDir)
		restorePrevious()
		return ErrorResult(fmt.Sprintf("downloaded skill %q is not valid", result.Slug))
	}

	if err := writeOriginMeta(targetDir, registry, result.Slug, installResult.Version); err != nil {
		logger.ErrorCF("tool", "Failed to write origin metadata", map[string]any{
			"tool":   "find_skill",
			"target": targetDir,
			"error":  err.Error(),
		})
	}
	if backupDir != "" {
		_ = os.RemoveAll(backupDir)
	}

	var output strings.Builder
	if installResult.IsSuspicious {
		output.WriteString(fmt.Sprintf("⚠️ Warning: skill %q is flagged as suspicious.\n\n", result.Slug))
	}
	output.WriteString(fmt.Sprintf("Successfully found and installed skill %q (alias: %s) from %s registry.\n",
		result.Slug, result.DisplayName, result.RegistryName))
	if installResult.Version != "" {
		output.WriteString(fmt.Sprintf("Version: %s\n", installResult.Version))
	}
	output.WriteString(fmt.Sprintf("Location: %s\n", targetDir))
	if installResult.Summary != "" {
		output.WriteString(fmt.Sprintf("Description: %s\n", installResult.Summary))
	}
	if result.Score > 0 {
		output.WriteString(fmt.Sprintf("Match score: %.3f\n", result.Score))
	}
	output.WriteString("\nThe skill is now available and can be loaded in the current session.")

	return SilentResult(output.String())
}
