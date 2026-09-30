package api

import (
	"encoding/json"
	"net/http"

	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/updater"
)

// registerUpdateRoutes registers the self-update endpoint.
func (h *Handler) registerUpdateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/update", h.handleUpdate)
	mux.HandleFunc("GET /api/update/check", h.handleUpdateCheck)
}

type updateRequest struct {
	URL    string `json:"url,omitempty"`
	Binary string `json:"binary,omitempty"`
}

type updateResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type updateCheckResponse struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	LatestURL       string `json:"latest_url,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
}

// handleUpdateCheck reports the latest GitHub release without downloading.
// The launcher WebUI polls this to show an "update available" badge.
func (h *Handler) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	st, err := updater.CheckForUpdate(config.GetVersion())
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(updateResponse{Status: "error", Message: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updateCheckResponse{
		Current:         st.Current,
		Latest:          st.Latest.TagName,
		LatestURL:       st.Latest.HTMLURL,
		UpdateAvailable: st.UpdateAvailable,
	})
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(updateResponse{Status: "error", Message: "method not allowed"})
		return
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var req updateRequest
	if err := dec.Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(updateResponse{Status: "error", Message: "invalid request body"})
		return
	}

	binary := req.Binary
	if binary == "" {
		binary = "clawy-launcher"
	}

	if err := updater.UpdateSelfFromRelease(req.URL, "", "", binary); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(updateResponse{Status: "error", Message: err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(updateResponse{Status: "ok", Message: "update applied; restart to use new version"})
}
