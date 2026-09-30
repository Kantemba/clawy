package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/cron"
)

// Job status values surfaced to the dashboard.
const (
	jobStatusDisabled  = "disabled"
	jobStatusError     = "error"
	jobStatusDue       = "due"
	jobStatusScheduled = "scheduled"
	jobStatusOK        = "ok"
)

// jobListItem is a cron job annotated with a UI friendly status.
type jobListItem struct {
	cron.CronJob
	Status string `json:"status"`
}

type jobListResponse struct {
	CronEnabled bool          `json:"cron_enabled"`
	Jobs        []jobListItem `json:"jobs"`
}

func (h *Handler) registerJobRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/jobs", h.handleListJobs)
}

func (h *Handler) handleListJobs(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	jobs, err := loadWorkspaceJobs(cfg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load jobs: %v", err), http.StatusInternalServerError)
		return
	}

	nowMS := time.Now().UnixMilli()
	items := make([]jobListItem, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, jobListItem{
			CronJob: job,
			Status:  jobDisplayStatus(job, nowMS),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobListResponse{
		CronEnabled: cfg.Tools.IsToolEnabled("cron"),
		Jobs:        items,
	})
}

// loadWorkspaceJobs reads the gateway cron store from the workspace.
// A missing store simply means no jobs have been scheduled yet.
func loadWorkspaceJobs(cfg *config.Config) ([]cron.CronJob, error) {
	storePath := filepath.Join(cfg.WorkspacePath(), "cron", "jobs.json")
	data, err := os.ReadFile(storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var store cron.CronStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	return store.Jobs, nil
}

// jobDisplayStatus derives a single status label from the persisted job state.
func jobDisplayStatus(job cron.CronJob, nowMS int64) string {
	switch {
	case !job.Enabled:
		return jobStatusDisabled
	case job.State.LastStatus == "error":
		return jobStatusError
	case job.State.NextRunAtMS != nil && *job.State.NextRunAtMS <= nowMS:
		return jobStatusDue
	case job.State.LastRunAtMS == nil:
		return jobStatusScheduled
	default:
		return jobStatusOK
	}
}
