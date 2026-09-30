package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Kantemba/clawy/pkg/agent"
	"github.com/Kantemba/clawy/pkg/config"
	"github.com/Kantemba/clawy/pkg/identity"
)

// Match the route resolver's default agent selection without constructing a
// runtime (providers and tools must not be started by a dashboard request).
func identityWorkspace(cfg *config.Config) string {
	var selected *config.AgentConfig
	if len(cfg.Agents.List) > 0 {
		selected = &cfg.Agents.List[0]
		for i := range cfg.Agents.List {
			if cfg.Agents.List[i].Default {
				selected = &cfg.Agents.List[i]
				break
			}
		}
	}
	return agent.ResolveAgentWorkspace(selected, &cfg.Agents.Defaults)
}

func (h *Handler) registerIdentityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agent/identity", h.handleGetIdentity)
	mux.HandleFunc("PUT /api/agent/identity", h.handleSaveIdentity)
}

func (h *Handler) handleGetIdentity(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, "Failed to load configuration", http.StatusInternalServerError)
		return
	}
	profile, err := identity.Load(identityWorkspace(cfg))
	if err != nil {
		http.Error(w, "Failed to read agent identity", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

func (h *Handler) handleSaveIdentity(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	var profile identity.Profile
	if err := decoder.Decode(&profile); err != nil {
		http.Error(w, "Invalid identity JSON", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "Expected a single identity object", http.StatusBadRequest)
		return
	}
	if err := profile.Normalize(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, "Failed to load configuration", http.StatusInternalServerError)
		return
	}
	if err := identity.Save(identityWorkspace(cfg), profile); err != nil {
		http.Error(w, "Failed to save agent identity", http.StatusInternalServerError)
		return
	}
	profile.Configured = true
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}
