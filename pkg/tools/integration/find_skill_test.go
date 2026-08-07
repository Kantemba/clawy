package integrationtools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Kantemba/clawy/pkg/skills"
)

func TestFindSkillToolName(t *testing.T) {
	tool := NewFindSkillTool(skills.NewRegistryManager(), "", nil)
	assert.Equal(t, "find_skill", tool.Name())
}

func TestFindSkillToolMissingQuery(t *testing.T) {
	tool := NewFindSkillTool(skills.NewRegistryManager(), "", nil)
	result := tool.Execute(context.Background(), map[string]any{})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "query is required")
}

func TestFindSkillToolEmptyQuery(t *testing.T) {
	tool := NewFindSkillTool(skills.NewRegistryManager(), "", nil)
	result := tool.Execute(context.Background(), map[string]any{
		"query": "   ",
	})
	assert.True(t, result.IsError)
}

func TestFindSkillToolNoRegistries(t *testing.T) {
	tool := NewFindSkillTool(skills.NewRegistryManager(), "", nil)
	result := tool.Execute(context.Background(), map[string]any{
		"query": "github",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "search failed")
}

func TestFindSkillToolInvalidRegistry(t *testing.T) {
	tool := NewFindSkillTool(skills.NewRegistryManager(), "", nil)
	result := tool.Execute(context.Background(), map[string]any{
		"query":    "github",
		"registry": "nonexistent",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "registry")
}

func TestFindSkillToolDescription(t *testing.T) {
	tool := NewFindSkillTool(skills.NewRegistryManager(), "", nil)
	desc := tool.Description()
	assert.NotEmpty(t, desc)
	assert.Contains(t, desc, "regist")
}

func TestFindSkillToolParameters(t *testing.T) {
	tool := NewFindSkillTool(skills.NewRegistryManager(), "", nil)
	params := tool.Parameters()

	props, ok := params["properties"].(map[string]any)
	assert.True(t, ok)
	assert.Contains(t, props, "query")
	assert.Contains(t, props, "registry")
	assert.Contains(t, props, "version")
	assert.Contains(t, props, "force")
	assert.Contains(t, props, "limit")

	required, ok := params["required"].([]string)
	assert.True(t, ok)
	assert.Contains(t, required, "query")
}
