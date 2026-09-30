package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/identity"
)

func TestIdentityRoutes(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := "Your name is Clawy. Custom workspace rules."
	if err := os.WriteFile(filepath.Join(workspace, "AGENT.md"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	NewHandler(configPath).RegisterRoutes(mux)
	request := func(method, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(method, "/api/agent/identity", strings.NewReader(body)))
		return rr
	}
	rr := request("GET", "")
	var profile identity.Profile
	if rr.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Configured || profile.Name != "Clawy" {
		t.Fatalf("unexpected default: %+v", profile)
	}

	rr = request("PUT", `{"name":" Nova ","avatar":"🦊","role":"Coding partner","personality":"Curious","instructions":"Use examples","greeting":"Let's build!","configured":false}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rr.Code, rr.Body.String())
	}
	// A new handler and GET prove the identity is persisted, not held in memory.
	fresh := httptest.NewRecorder()
	NewHandler(configPath).handleGetIdentity(fresh, httptest.NewRequest("GET", "/api/agent/identity", nil))
	if err := json.Unmarshal(fresh.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if !profile.Configured || profile.Name != "Nova" || profile.Personality != "Curious" {
		t.Fatalf("unexpected saved profile: %+v", profile)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "AGENT.md"))
	if err != nil || string(data) != legacy {
		t.Fatal("identity save modified existing workspace instructions")
	}

	for _, body := range []string{`{"name":""}`, `{"name":"a\nb"}`, `{"name":"Nova","unknown":true}`, `{"name":"Nova"} {}`, `null`, `{"name":"Nova","instructions":"` + strings.Repeat("a", 8001) + `"}`} {
		if rr := request("PUT", body); rr.Code != http.StatusBadRequest {
			t.Fatalf("invalid identity accepted: %d", rr.Code)
		}
	}
	profile, err = identity.Load(workspace)
	if err != nil || profile.Name != "Nova" {
		t.Fatal("invalid request changed saved identity")
	}
}

func TestIdentityWorkspaceUsesDefaultAgent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(t.TempDir(), "workspace")
	customWorkspace := filepath.Join(t.TempDir(), "nova-workspace")
	cfg.Agents.List = []config.AgentConfig{
		{ID: "other"},
		{ID: "nova", Default: true, Workspace: customWorkspace},
	}
	if workspace := identityWorkspace(cfg); workspace != customWorkspace {
		t.Fatalf("identity workspace = %q, want %q", workspace, customWorkspace)
	}
	cfg.Agents.List = []config.AgentConfig{{ID: "nova"}}
	want := filepath.Join(cfg.Agents.Defaults.Workspace, "..", "workspace-nova")
	if workspace := identityWorkspace(cfg); workspace != want {
		t.Fatalf("named agent workspace = %q, want %q", workspace, want)
	}
}
