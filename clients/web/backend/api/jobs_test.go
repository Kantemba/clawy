package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/cron"
)

func TestHandleListJobsEmptyStore(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Tools.Cron.Enabled = true
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	rec := doListJobs(t, configPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp jobListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !resp.CronEnabled {
		t.Fatal("cron_enabled = false, want true")
	}
	if len(resp.Jobs) != 0 {
		t.Fatalf("len(jobs) = %d, want 0", len(resp.Jobs))
	}
}

func TestHandleListJobsReportsStatus(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Tools.Cron.Enabled = false

	workspace := t.TempDir()
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	past := int64(1)
	future := int64(1 << 62)
	lastRun := int64(42)
	store := cron.CronStore{
		Version: 1,
		Jobs: []cron.CronJob{
			{
				ID:      "disabled",
				Name:    "Paused",
				Enabled: false,
			},
			{
				ID:      "failed",
				Name:    "Broken",
				Enabled: true,
				State:   cron.CronJobState{LastStatus: "error", LastError: "boom"},
			},
			{
				ID:      "due",
				Name:    "Due",
				Enabled: true,
				State:   cron.CronJobState{NextRunAtMS: &past, LastRunAtMS: &lastRun, LastStatus: "ok"},
			},
			{
				ID:      "fresh",
				Name:    "Fresh",
				Enabled: true,
				State:   cron.CronJobState{NextRunAtMS: &future},
			},
			{
				ID:      "healthy",
				Name:    "Healthy",
				Enabled: true,
				State:   cron.CronJobState{NextRunAtMS: &future, LastRunAtMS: &lastRun, LastStatus: "ok"},
			},
		},
	}
	data, err := json.Marshal(store)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	storeDir := filepath.Join(workspace, "cron")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "jobs.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rec := doListJobs(t, configPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp jobListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.CronEnabled {
		t.Fatal("cron_enabled = true, want false")
	}
	if len(resp.Jobs) != len(store.Jobs) {
		t.Fatalf("len(jobs) = %d, want %d", len(resp.Jobs), len(store.Jobs))
	}

	got := make(map[string]string, len(resp.Jobs))
	for _, job := range resp.Jobs {
		got[job.ID] = job.Status
	}
	want := map[string]string{
		"disabled": jobStatusDisabled,
		"failed":   jobStatusError,
		"due":      jobStatusDue,
		"fresh":    jobStatusScheduled,
		"healthy":  jobStatusOK,
	}
	for id, status := range want {
		if got[id] != status {
			t.Errorf("job %q status = %q, want %q", id, got[id], status)
		}
	}
}

func TestHandleListJobsInvalidStore(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	workspace := t.TempDir()
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	storeDir := filepath.Join(workspace, "cron")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "jobs.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rec := doListJobs(t, configPath)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func doListJobs(t *testing.T, configPath string) *httptest.ResponseRecorder {
	t.Helper()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/jobs", nil))
	return rec
}
