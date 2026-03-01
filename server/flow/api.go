package flow

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const maxRequestBodySize = 1 << 20 // 1 MB

// APIHandler provides HTTP handlers for flow CRUD operations.
type APIHandler struct {
	store model.Store
	api   plugin.API
}

// NewAPIHandler creates a new flow API handler.
func NewAPIHandler(store model.Store, api plugin.API) *APIHandler {
	return &APIHandler{store: store, api: api}
}

// RegisterRoutes registers the flow CRUD routes on the given router.
func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/flows", h.handleListFlows).Methods(http.MethodGet)
	r.HandleFunc("/flows", h.handleCreateFlow).Methods(http.MethodPost)
	r.HandleFunc("/flows/{id}", h.handleGetFlow).Methods(http.MethodGet)
	r.HandleFunc("/flows/{id}", h.handleUpdateFlow).Methods(http.MethodPut)
	r.HandleFunc("/flows/{id}", h.handleDeleteFlow).Methods(http.MethodDelete)
}

func (h *APIHandler) handleListFlows(w http.ResponseWriter, r *http.Request) {
	flows, err := h.store.List()
	if err != nil {
		h.api.LogError("Failed to list flows", "error", err.Error())
		http.Error(w, "failed to list flows", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(flows); err != nil {
		h.api.LogError("Failed to encode flows", "error", err.Error())
	}
}

func (h *APIHandler) handleCreateFlow(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var f model.Flow
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	f.ID = mmmodel.NewId()
	f.CreatedAt = time.Now().UnixMilli()
	f.UpdatedAt = f.CreatedAt
	f.CreatedBy = r.Header.Get("Mattermost-User-ID")

	// Assign IDs to actions that don't have one.
	for i := range f.Actions {
		if f.Actions[i].ID == "" {
			f.Actions[i].ID = mmmodel.NewId()
		}
	}

	if err := h.store.Save(&f); err != nil {
		h.api.LogError("Failed to create flow", "error", err.Error())
		http.Error(w, "failed to create flow", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(&f); err != nil {
		h.api.LogError("Failed to encode flow", "error", err.Error())
	}
}

func (h *APIHandler) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	f, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get flow", "error", err.Error())
		http.Error(w, "failed to get flow", http.StatusInternalServerError)
		return
	}
	if f == nil {
		http.Error(w, "flow not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(f); err != nil {
		h.api.LogError("Failed to encode flow", "error", err.Error())
	}
}

func (h *APIHandler) handleUpdateFlow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	existing, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get flow for update", "error", err.Error())
		http.Error(w, "failed to get flow", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, "flow not found", http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var f model.Flow
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Preserve immutable fields.
	f.ID = id
	f.CreatedAt = existing.CreatedAt
	f.CreatedBy = existing.CreatedBy
	f.UpdatedAt = time.Now().UnixMilli()

	// Assign IDs to actions that don't have one.
	for i := range f.Actions {
		if f.Actions[i].ID == "" {
			f.Actions[i].ID = mmmodel.NewId()
		}
	}

	if err := h.store.Save(&f); err != nil {
		h.api.LogError("Failed to update flow", "error", err.Error())
		http.Error(w, "failed to update flow", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&f); err != nil {
		h.api.LogError("Failed to encode flow", "error", err.Error())
	}
}

func (h *APIHandler) handleDeleteFlow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	if err := h.store.Delete(id); err != nil {
		h.api.LogError("Failed to delete flow", "error", err.Error())
		http.Error(w, "failed to delete flow", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
