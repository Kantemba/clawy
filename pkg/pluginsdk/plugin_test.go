package pluginsdk

import (
	"context"
	"testing"
)

type testPlugin struct {
	BasePlugin
	initCalled     bool
	shutdownCalled bool
}

func (p *testPlugin) Metadata() PluginMeta {
	return PluginMeta{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "A test plugin",
		Author:      "test",
	}
}

func (p *testPlugin) Init(host Host) error {
	p.initCalled = true
	return nil
}

func (p *testPlugin) Shutdown() error {
	p.shutdownCalled = true
	return nil
}

type testToolPlugin struct {
	BasePlugin
	tools []ToolDefinition
}

func (p *testToolPlugin) Metadata() PluginMeta {
	return PluginMeta{
		Name:        "test-tool-plugin",
		Version:     "1.0.0",
		Description: "A test tool plugin",
	}
}

func (p *testToolPlugin) Tools() []ToolDefinition {
	return p.tools
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		plugin  Plugin
		wantErr bool
	}{
		{
			"valid plugin",
			&testPlugin{},
			false,
		},
		{
			"missing name",
			&badPlugin{name: ""},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.plugin)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type badPlugin struct {
	BasePlugin
	name string
}

func (p *badPlugin) Metadata() PluginMeta {
	return PluginMeta{Name: p.name, Version: "1.0.0", Description: "test"}
}

func TestManagerRegister(t *testing.T) {
	adapter := NewAdapter(context.Background(), nil, nil, nil)
	mgr := NewManager(adapter, "")

	plugin := &testPlugin{}
	if err := mgr.Register(plugin); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if !plugin.initCalled {
		t.Error("Init() was not called")
	}

	plugins := mgr.Plugins()
	if len(plugins) != 1 {
		t.Errorf("Plugins() = %d, want 1", len(plugins))
	}

	if err := mgr.Unregister("test-plugin"); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	if !plugin.shutdownCalled {
		t.Error("Shutdown() was not called")
	}
}

func TestManagerToolPlugin(t *testing.T) {
	adapter := NewAdapter(context.Background(), nil, nil, nil)
	mgr := NewManager(adapter, "")

	toolDef := ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, args map[string]any) *ToolResult {
			return NewResult("ok")
		},
	}

	plugin := &testToolPlugin{tools: []ToolDefinition{toolDef}}
	if err := mgr.Register(plugin); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	def, ok := mgr.GetTool("test_tool")
	if !ok {
		t.Fatal("GetTool() not found")
	}

	if def.Name != "test_tool" {
		t.Errorf("tool name = %s, want test_tool", def.Name)
	}

	tools := mgr.GetTools()
	if len(tools) != 1 {
		t.Errorf("GetTools() = %d, want 1", len(tools))
	}
}

func TestToolResult(t *testing.T) {
	r := NewResult("hello")
	if r.IsError {
		t.Error("NewResult should not be error")
	}
	if r.ContentForLLM != "hello" {
		t.Errorf("ContentForLLM = %s, want hello", r.ContentForLLM)
	}

	e := NewErrorResult("fail")
	if !e.IsError {
		t.Error("NewErrorResult should be error")
	}
}
