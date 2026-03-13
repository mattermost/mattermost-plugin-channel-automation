package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const maxRequestBodySize = 1 << 20 // 1 MB

// writeErrorJSON writes an error response as JSON that the Mattermost client can parse.
func writeErrorJSON(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":          "channel-automation.error",
		"message":     message,
		"status_code": statusCode,
	})
}

// APIHandler provides HTTP handlers for flow CRUD operations.
type APIHandler struct {
	store           model.Store
	historyStore    model.ExecutionStore
	api             plugin.API
	scheduleManager *ScheduleManager
	config          model.Configuration
}

// NewAPIHandler creates a new flow API handler.
func NewAPIHandler(store model.Store, historyStore model.ExecutionStore, api plugin.API, scheduleManager *ScheduleManager, config model.Configuration) *APIHandler {
	return &APIHandler{store: store, historyStore: historyStore, api: api, scheduleManager: scheduleManager, config: config}
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
// (SchemeAdmin) on every literal channel referenced in the flow.
//
// When no concrete channels can be verified (e.g. a channel_created trigger
// with only templated or AI-only actions), we require system admin permission.
// The authorization model relies on proving the user is admin on every channel
// the flow touches. If there are zero channels to verify, we have no evidence
// the user should be allowed to manage this flow, so we deny rather than
// silently granting access to what is effectively a global-scope operation.
func (h *APIHandler) checkFlowPermissions(userID string, f *model.Flow) error {
	if h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
		return nil
	}
	channelIDs := model.CollectChannelIDs(f)
	if len(channelIDs) == 0 {
		return fmt.Errorf("system admin permission is required for flows without explicit channel references")
	}
	for _, chID := range channelIDs {
		member, appErr := h.api.GetChannelMember(chID, userID)
		if appErr != nil {
			// 4xx errors (not found, unauthorized) mean the user is not a member;
			// 5xx errors are infrastructure failures that should surface differently.
			if appErr.StatusCode >= http.StatusInternalServerError {
				return fmt.Errorf("failed to verify channel permissions: %w", appErr)
			}
			return fmt.Errorf("you do not have channel admin permissions on one or more channels referenced by this flow")
		}
		if !member.SchemeAdmin {
			return fmt.Errorf("you do not have channel admin permissions on one or more channels referenced by this flow")
		}
	}
	return nil
}

// writePermissionError writes the appropriate HTTP error based on whether
// the permission check failed due to an API/infrastructure error (500) or
// the user genuinely lacking permissions (403).
func (h *APIHandler) writePermissionError(w http.ResponseWriter, err error) {
	var appErr *mmmodel.AppError
	if errors.As(err, &appErr) {
		h.api.LogError("Failed to verify permissions", "error", err.Error())
		http.Error(w, "failed to verify permissions", http.StatusInternalServerError)
		return
	}
	http.Error(w, err.Error(), http.StatusForbidden)
}

// checkChannelFlowLimit verifies that the channel has not reached the
// per-channel flow limit. excludeFlowID is used during updates so the
// flow being modified does not count against itself.
func (h *APIHandler) checkChannelFlowLimit(channelID, excludeFlowID string) error {
	if channelID == "" {
		return nil
	}

	limit := 0
	if h.config != nil {
		limit = h.config.MaxFlowsPerChannel()
	}
	if limit <= 0 {
		return nil
	}

	count, err := h.store.CountByTriggerChannel(channelID)
	if err != nil {
		return fmt.Errorf("failed to check channel flow count: %w", err)
	}

	if excludeFlowID != "" {
		existing, err := h.store.Get(excludeFlowID)
		if err != nil {
			return fmt.Errorf("failed to check existing flow: %w", err)
		}
		if existing != nil && existing.TriggerChannelID() == channelID {
			count--
		}
	}

	if count >= limit {
		return fmt.Errorf("channel has reached the maximum of %d flow(s)", limit)
	}
	return nil
}

func (h *APIHandler) handleListFlows(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		writeErrorJSON(w, "missing user ID", http.StatusUnauthorized)
		return
	}

	var flows []*model.Flow
	var err error
	if channelID := r.URL.Query().Get("channel_id"); channelID != "" {
		flows, err = h.store.ListByTriggerChannel(channelID)
	} else {
		flows, err = h.store.List()
	}
	if err != nil {
		h.api.LogError("Failed to list flows", "error", err.Error())
		writeErrorJSON(w, "failed to list flows", http.StatusInternalServerError)
		return
	}

	// Filter flows to only those the user has permission to view.
	isAdmin := h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem)
	if !isAdmin {
		visible := make([]*model.Flow, 0, len(flows))
		for _, f := range flows {
			if h.checkFlowPermissions(userID, f) == nil {
				visible = append(visible, f)
			}
		}
		flows = visible
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
		writeErrorJSON(w, "invalid request body", http.StatusBadRequest)
		return
	}

	f.ID = mmmodel.NewId()
	f.CreatedAt = time.Now().UnixMilli()
	f.UpdatedAt = f.CreatedAt
	f.CreatedBy = r.Header.Get("Mattermost-User-ID")
	if f.CreatedBy == "" {
		writeErrorJSON(w, "missing user ID", http.StatusUnauthorized)
		return
	}

	if err := model.ValidateActions(f.Actions); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := model.ValidateTrigger(&f.Trigger, nil); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.checkFlowPermissions(f.CreatedBy, &f); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusForbidden)
		h.writePermissionError(w, err)
		return
	}

	if err := h.checkChannelFlowLimit(f.TriggerChannelID(), ""); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusConflict)
		return
	}

	if err := h.store.Save(&f); err != nil {
		h.api.LogError("Failed to create flow", "error", err.Error())
		writeErrorJSON(w, "failed to create flow", http.StatusInternalServerError)
		return
	}

	if h.scheduleManager != nil {
		if err := h.scheduleManager.SyncFlow(nil, &f); err != nil {
			h.api.LogWarn("Failed to sync schedule after flow create", "flow_id", f.ID, "error", err.Error())
		}
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
		writeErrorJSON(w, "failed to get flow", http.StatusInternalServerError)
		return
	}
	if f == nil {
		writeErrorJSON(w, "flow not found", http.StatusNotFound)
		return
	}

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		writeErrorJSON(w, "missing user ID", http.StatusUnauthorized)
		return
	}

	if err := h.checkFlowPermissions(userID, f); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusForbidden)
		h.writePermissionError(w, err)
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
		writeErrorJSON(w, "failed to get flow", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		writeErrorJSON(w, "flow not found", http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var f model.Flow
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		writeErrorJSON(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Preserve immutable fields.
	f.ID = id
	f.CreatedAt = existing.CreatedAt
	f.CreatedBy = existing.CreatedBy
	f.UpdatedAt = time.Now().UnixMilli()

	if err := model.ValidateActions(f.Actions); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := model.ValidateTrigger(&f.Trigger, &existing.Trigger); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		writeErrorJSON(w, "missing user ID", http.StatusUnauthorized)
		return
	}

	// Check permissions on existing flow (must have permission to modify it).
	if err := h.checkFlowPermissions(userID, existing); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusForbidden)
		h.writePermissionError(w, err)
		return
	}

	// Check permissions on new flow configuration (must have permission on new channels).
	if err := h.checkFlowPermissions(userID, &f); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusForbidden)
		h.writePermissionError(w, err)
		return
	}

	if err := h.checkChannelFlowLimit(f.TriggerChannelID(), f.ID); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusConflict)
		return
	}

	if err := h.store.Save(&f); err != nil {
		h.api.LogError("Failed to update flow", "error", err.Error())
		writeErrorJSON(w, "failed to update flow", http.StatusInternalServerError)
		return
	}

	if h.scheduleManager != nil {
		if err := h.scheduleManager.SyncFlow(existing, &f); err != nil {
			h.api.LogWarn("Failed to sync schedule after flow update", "flow_id", f.ID, "error", err.Error())
		}
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
		writeErrorJSON(w, "failed to get flow", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		writeErrorJSON(w, "flow not found", http.StatusNotFound)
		return
	}

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		writeErrorJSON(w, "missing user ID", http.StatusUnauthorized)
		return
	}

	if err := h.checkFlowPermissions(userID, existing); err != nil {
		writeErrorJSON(w, err.Error(), http.StatusForbidden)
		h.writePermissionError(w, err)
		return
	}

	if err := h.store.Delete(id); err != nil {
		h.api.LogError("Failed to delete flow", "error", err.Error())
		writeErrorJSON(w, "failed to delete flow", http.StatusInternalServerError)
		return
	}

	if h.scheduleManager != nil {
		h.scheduleManager.RemoveFlow(id)
	}

	if h.historyStore != nil {
		if err := h.historyStore.PurgeFlow(id); err != nil {
			h.api.LogError("Failed to purge execution history", "flow_id", id, "error", err.Error())
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
