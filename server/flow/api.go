package flow

import (
	"encoding/json"
	"fmt"
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
	store           model.Store
	api             plugin.API
	scheduleManager *ScheduleManager
}

// NewAPIHandler creates a new flow API handler.
func NewAPIHandler(store model.Store, api plugin.API, scheduleManager *ScheduleManager) *APIHandler {
	return &APIHandler{store: store, api: api, scheduleManager: scheduleManager}
}

// RegisterRoutes registers the flow CRUD routes on the given router.
func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/flows", h.handleListFlows).Methods(http.MethodGet)
	r.HandleFunc("/flows", h.handleCreateFlow).Methods(http.MethodPost)
	r.HandleFunc("/flows/{id}", h.handleGetFlow).Methods(http.MethodGet)
	r.HandleFunc("/flows/{id}", h.handleUpdateFlow).Methods(http.MethodPut)
	r.HandleFunc("/flows/{id}", h.handleDeleteFlow).Methods(http.MethodDelete)
}

// checkFlowPermissions verifies that userID has permission to manage the flow.
// System admins are always allowed. Otherwise the user must be a channel admin
// (SchemeAdmin) on every literal channel referenced in the flow. It returns the
// first failing channel ID and false when the check fails.
func (h *APIHandler) checkFlowPermissions(userID string, f *model.Flow) (string, bool) {
	if h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
		return "", true
	}
	for _, chID := range model.CollectChannelIDs(f) {
		member, appErr := h.api.GetChannelMember(chID, userID)
		if appErr != nil || !member.SchemeAdmin {
			return chID, false
		}
	}
	return "", true
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

	if err := model.ValidateTrigger(&f.Trigger); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if chID, ok := h.checkFlowPermissions(f.CreatedBy, &f); !ok {
		http.Error(w, fmt.Sprintf("you do not have channel admin permissions on channel %s", chID), http.StatusForbidden)
		return
	}

	if err := h.store.Save(&f); err != nil {
		h.api.LogError("Failed to create flow", "error", err.Error())
		http.Error(w, "failed to create flow", http.StatusInternalServerError)
		return
	}

	if h.scheduleManager != nil {
		h.scheduleManager.SyncFlow(nil, &f)
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

	if err := model.ValidateTrigger(&f.Trigger); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, "missing user ID", http.StatusUnauthorized)
		return
	}

	// Check permissions on existing flow (must have permission to modify it).
	if chID, ok := h.checkFlowPermissions(userID, existing); !ok {
		http.Error(w, fmt.Sprintf("you do not have channel admin permissions on channel %s", chID), http.StatusForbidden)
		return
	}

	// Check permissions on new flow configuration (must have permission on new channels).
	if chID, ok := h.checkFlowPermissions(userID, &f); !ok {
		http.Error(w, fmt.Sprintf("you do not have channel admin permissions on channel %s", chID), http.StatusForbidden)
		return
	}

	if err := h.store.Save(&f); err != nil {
		h.api.LogError("Failed to update flow", "error", err.Error())
		http.Error(w, "failed to update flow", http.StatusInternalServerError)
		return
	}

	if h.scheduleManager != nil {
		h.scheduleManager.SyncFlow(existing, &f)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&f); err != nil {
		h.api.LogError("Failed to encode flow", "error", err.Error())
	}
}

func (h *APIHandler) handleDeleteFlow(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	existing, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get flow for delete", "error", err.Error())
		http.Error(w, "failed to get flow", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, "flow not found", http.StatusNotFound)
		return
	}

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, "missing user ID", http.StatusUnauthorized)
		return
	}

	if chID, ok := h.checkFlowPermissions(userID, existing); !ok {
		http.Error(w, fmt.Sprintf("you do not have channel admin permissions on channel %s", chID), http.StatusForbidden)
		return
	}

	if err := h.store.Delete(id); err != nil {
		h.api.LogError("Failed to delete flow", "error", err.Error())
		http.Error(w, "failed to delete flow", http.StatusInternalServerError)
		return
	}

	if h.scheduleManager != nil {
		h.scheduleManager.RemoveFlow(id)
	}

	w.WriteHeader(http.StatusNoContent)
}
