package automation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/httputil"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/permissions"
)

const maxRequestBodySize = 1 << 20 // 1 MB

// APIHandler provides HTTP handlers for automation CRUD operations.
type APIHandler struct {
	store           model.Store
	historyStore    model.ExecutionStore
	api             plugin.API
	scheduleManager *ScheduleManager
	config          model.Configuration
}

// NewAPIHandler creates a new automation API handler.
func NewAPIHandler(store model.Store, historyStore model.ExecutionStore, api plugin.API, scheduleManager *ScheduleManager, config model.Configuration) *APIHandler {
	return &APIHandler{store: store, historyStore: historyStore, api: api, scheduleManager: scheduleManager, config: config}
}

// RegisterRoutes registers the automation CRUD routes on the given router.
func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/automations", h.handleListAutomations).Methods(http.MethodGet)
	r.HandleFunc("/automations", h.handleCreateAutomation).Methods(http.MethodPost)
	r.HandleFunc("/automations/{id}", h.handleGetAutomation).Methods(http.MethodGet)
	r.HandleFunc("/automations/{id}", h.handleUpdateAutomation).Methods(http.MethodPut)
	r.HandleFunc("/automations/{id}", h.handleDeleteAutomation).Methods(http.MethodDelete)
}

// checkChannelAutomationLimit verifies that the channel has not reached the
// per-channel automation limit. excludeAutomationID is used during updates so the
// automation being modified does not count against itself.
func (h *APIHandler) checkChannelAutomationLimit(channelID, excludeAutomationID string) error {
	if channelID == "" {
		return nil
	}

	limit := 0
	if h.config != nil {
		limit = h.config.MaxAutomationsPerChannel()
	}
	if limit <= 0 {
		return nil
	}

	count, err := h.store.CountByTriggerChannel(channelID)
	if err != nil {
		return fmt.Errorf("failed to check channel automation count: %w", err)
	}

	if excludeAutomationID != "" {
		existing, err := h.store.Get(excludeAutomationID)
		if err != nil {
			return fmt.Errorf("failed to check existing automation: %w", err)
		}
		if existing != nil && existing.TriggerChannelID() == channelID {
			count--
		}
	}

	if count >= limit {
		return fmt.Errorf("channel has reached the maximum of %d automation(s)", limit)
	}
	return nil
}

func (h *APIHandler) handleListAutomations(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	var automations []*model.Automation
	var err error
	if channelID := r.URL.Query().Get("channel_id"); channelID != "" {
		automations, err = h.store.ListByTriggerChannel(channelID)
	} else {
		automations, err = h.store.List()
	}
	if err != nil {
		h.api.LogError("Failed to list automations", "user_id", userID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to list automations", "")
		return
	}

	// Filter automations to only those the user has permission to view.
	isAdmin := h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem)
	if !isAdmin {
		visible := make([]*model.Automation, 0, len(automations))
		for _, a := range automations {
			if permissions.CheckAutomationPermissions(h.api, userID, a) == nil {
				visible = append(visible, a)
			}
		}
		automations = visible
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(automations); err != nil {
		h.api.LogError("Failed to encode automations", "error", err.Error())
	}
}

func (h *APIHandler) handleCreateAutomation(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var a model.Automation
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "invalid request body", "")
		return
	}

	a.ID = mmmodel.NewId()
	a.CreatedAt = time.Now().UnixMilli()
	a.UpdatedAt = a.CreatedAt
	a.CreatedBy = userID

	if a.Name == "" {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "name is required", "")
		return
	}
	if len(a.Name) > 100 {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "name must be 100 characters or fewer", "")
		return
	}

	if err := model.ValidateTrigger(&a.Trigger, nil); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	if err := model.ValidateActions(a.Actions); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	if err := model.ValidateSendMessageChannel(&a); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	if err := permissions.CheckAutomationPermissions(h.api, a.CreatedBy, &a); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, a.CreatedBy, a.ID)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	if err := h.checkChannelAutomationLimit(a.TriggerChannelID(), ""); err != nil {
		httputil.WriteErrorJSON(w, http.StatusConflict, err.Error(), "")
		return
	}

	if err := h.store.Save(&a); err != nil {
		h.api.LogError("Failed to create automation", "user_id", a.CreatedBy, "automation_id", a.ID, "automation_name", a.Name, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to create automation", "")
		return
	}

	if h.scheduleManager != nil {
		if err := h.scheduleManager.SyncAutomation(nil, &a); err != nil {
			h.api.LogWarn("Failed to sync schedule after automation create", "automation_id", a.ID, "error", err.Error())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(&a); err != nil {
		h.api.LogError("Failed to encode automation", "error", err.Error())
	}
}

func (h *APIHandler) handleGetAutomation(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	id := mux.Vars(r)["id"]

	a, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get automation", "automation_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get automation", "")
		return
	}
	if a == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "automation not found", "")
		return
	}

	if err := permissions.CheckAutomationPermissions(h.api, userID, a); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, userID, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a); err != nil {
		h.api.LogError("Failed to encode automation", "error", err.Error())
	}
}

func (h *APIHandler) handleUpdateAutomation(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	id := mux.Vars(r)["id"]

	existing, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get automation for update", "automation_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get automation", "")
		return
	}
	if existing == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "automation not found", "")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var a model.Automation
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "invalid request body", "")
		return
	}

	// Preserve immutable fields.
	a.ID = id
	a.CreatedAt = existing.CreatedAt
	a.CreatedBy = existing.CreatedBy
	a.UpdatedAt = time.Now().UnixMilli()

	if a.Name == "" {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "name is required", "")
		return
	}
	if len(a.Name) > 100 {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "name must be 100 characters or fewer", "")
		return
	}

	if err := model.ValidateActions(a.Actions); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	if err := model.ValidateSendMessageChannel(&a); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	if err := model.ValidateTrigger(&a.Trigger, &existing.Trigger); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	// Check permissions on existing automation (must have permission to modify it).
	if err := permissions.CheckAutomationPermissions(h.api, userID, existing); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, userID, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	// Check permissions on new automation configuration (must have permission on new channels).
	if err := permissions.CheckAutomationPermissions(h.api, userID, &a); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, userID, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	if err := h.checkChannelAutomationLimit(a.TriggerChannelID(), a.ID); err != nil {
		httputil.WriteErrorJSON(w, http.StatusConflict, err.Error(), "")
		return
	}

	if err := h.store.Save(&a); err != nil {
		h.api.LogError("Failed to update automation", "user_id", userID, "automation_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to update automation", "")
		return
	}

	if h.scheduleManager != nil {
		if err := h.scheduleManager.SyncAutomation(existing, &a); err != nil {
			h.api.LogWarn("Failed to sync schedule after automation update", "automation_id", a.ID, "error", err.Error())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&a); err != nil {
		h.api.LogError("Failed to encode automation", "error", err.Error())
	}
}

func (h *APIHandler) handleDeleteAutomation(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	id := mux.Vars(r)["id"]

	existing, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get automation for delete", "automation_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get automation", "")
		return
	}
	if existing == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "automation not found", "")
		return
	}

	if err := permissions.CheckAutomationPermissions(h.api, userID, existing); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, userID, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	if err := h.store.Delete(id); err != nil {
		h.api.LogError("Failed to delete automation", "user_id", userID, "automation_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to delete automation", "")
		return
	}

	if h.scheduleManager != nil {
		h.scheduleManager.RemoveAutomation(id)
	}

	if h.historyStore != nil {
		if err := h.historyStore.PurgeAutomation(id); err != nil {
			h.api.LogError("Failed to purge execution history", "user_id", userID, "automation_id", id, "error", err.Error())
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
