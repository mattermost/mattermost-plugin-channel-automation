package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation/hooks"
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
	registry        *Registry
	scheduleManager *ScheduleManager
	config          model.Configuration
	bridge          hooks.AgentToolsLister
}

// NewAPIHandler creates a new automation API handler. bridge may be nil in tests
// that do not exercise allowed_tools validation.
func NewAPIHandler(store model.Store, historyStore model.ExecutionStore, api plugin.API, registry *Registry, scheduleManager *ScheduleManager, config model.Configuration, bridge hooks.AgentToolsLister) *APIHandler {
	return &APIHandler{store: store, historyStore: historyStore, api: api, registry: registry, scheduleManager: scheduleManager, config: config, bridge: bridge}
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
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to list automations", err.Error())
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
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var f model.Automation
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "invalid request body", "")
		return
	}

	f.ID = mmmodel.NewId()
	f.CreatedAt = model.NowTimestamp()
	f.UpdatedAt = f.CreatedAt
	f.CreatedBy = r.Header.Get("Mattermost-User-ID")
	if f.CreatedBy == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	if f.Name == "" {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "name is required", "")
		return
	}
	if len(f.Name) > 100 {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "name must be 100 characters or fewer", "")
		return
	}

	if err := ValidateTrigger(h.registry, &f.Trigger, nil); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	if err := model.ValidateActions(f.Actions); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	if err := model.ValidateSendMessageChannel(&f); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	if err := permissions.CheckAutomationPermissions(h.api, f.CreatedBy, &f); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, f.CreatedBy, f.ID)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	if err := permissions.CheckGuardrailChannelPermissions(h.api, f.CreatedBy, &f); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, f.CreatedBy, f.ID)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	// Bridge-backed agent access verification runs after the local permission
	// checks so we never call the bridge for automations the user cannot manage.
	if err := hooks.ValidateAllowedTools(&f, f.CreatedBy, h.bridge); err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, hooks.ErrToolDiscovery) {
			code = http.StatusBadGateway
		}
		httputil.WriteErrorJSON(w, code, err.Error(), "")
		return
	}

	if err := permissions.CheckGuardrailsRequired(h.api, &f); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, f.CreatedBy, f.ID)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	if err := h.checkChannelAutomationLimit(f.TriggerChannelID(), ""); err != nil {
		httputil.WriteErrorJSON(w, http.StatusConflict, err.Error(), "")
		return
	}

	if err := h.store.Save(&f); err != nil {
		h.api.LogError("Failed to create automation", "user_id", f.CreatedBy, "automation_id", f.ID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to create automation", err.Error())
		return
	}

	if h.scheduleManager != nil {
		if err := h.scheduleManager.SyncAutomation(nil, &f); err != nil {
			h.api.LogWarn("Failed to sync schedule after automation create", "automation_id", f.ID, "error", err.Error())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(&f); err != nil {
		h.api.LogError("Failed to encode automation", "error", err.Error())
	}
}

func (h *APIHandler) handleGetAutomation(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	f, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get automation", "automation_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get automation", err.Error())
		return
	}
	if f == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "automation not found", "")
		return
	}

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	if err := permissions.CheckAutomationPermissions(h.api, userID, f); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, userID, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(f); err != nil {
		h.api.LogError("Failed to encode automation", "error", err.Error())
	}
}

func (h *APIHandler) handleUpdateAutomation(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	existing, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get automation for update", "automation_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get automation", err.Error())
		return
	}
	if existing == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "automation not found", "")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var f model.Automation
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "invalid request body", "")
		return
	}

	// Preserve immutable fields.
	f.ID = id
	f.CreatedAt = existing.CreatedAt
	f.CreatedBy = existing.CreatedBy
	f.UpdatedAt = model.NowTimestamp()

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	// Only the creator (or a sysadmin acting on their behalf) may edit an automation.
	// This is the security boundary that lets the downstream creator-anchored
	// checks below validate against existing.CreatedBy without enabling
	// privilege escalation by a non-creator editor.
	if err := permissions.CanEditAutomation(h.api, userID, existing); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, userID, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	if f.Name == "" {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "name is required", "")
		return
	}
	if len(f.Name) > 100 {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, "name must be 100 characters or fewer", "")
		return
	}

	if err := model.ValidateActions(f.Actions); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	if err := model.ValidateSendMessageChannel(&f); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	if err := ValidateTrigger(h.registry, &f.Trigger, &existing.Trigger); err != nil {
		httputil.WriteErrorJSON(w, http.StatusBadRequest, err.Error(), "")
		return
	}

	// New automation configuration must remain admissible for the creator (covers
	// the sysadmin-edit case: catches references the creator cannot manage).
	if err := permissions.CheckAutomationPermissions(h.api, existing.CreatedBy, &f); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, existing.CreatedBy, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	if err := permissions.CheckGuardrailChannelPermissions(h.api, existing.CreatedBy, &f); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, existing.CreatedBy, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	// Bridge-backed agent access verification uses the original creator's
	// identity (matches the runtime model where the bridge ACL is checked
	// against created_by, not the editor) and runs after the local permission
	// checks so we never call the bridge for inadmissible automations.
	if err := hooks.ValidateAllowedTools(&f, f.CreatedBy, h.bridge); err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, hooks.ErrToolDiscovery) {
			code = http.StatusBadGateway
		}
		httputil.WriteErrorJSON(w, code, err.Error(), "")
		return
	}

	if err := permissions.CheckGuardrailsRequired(h.api, &f); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, existing.CreatedBy, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	if err := h.checkChannelAutomationLimit(f.TriggerChannelID(), f.ID); err != nil {
		httputil.WriteErrorJSON(w, http.StatusConflict, err.Error(), "")
		return
	}

	if err := h.store.Save(&f); err != nil {
		h.api.LogError("Failed to update automation", "user_id", userID, "automation_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to update automation", err.Error())
		return
	}

	if h.scheduleManager != nil {
		if err := h.scheduleManager.SyncAutomation(existing, &f); err != nil {
			h.api.LogWarn("Failed to sync schedule after automation update", "automation_id", f.ID, "error", err.Error())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&f); err != nil {
		h.api.LogError("Failed to encode automation", "error", err.Error())
	}
}

func (h *APIHandler) handleDeleteAutomation(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	existing, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get automation for delete", "automation_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get automation", err.Error())
		return
	}
	if existing == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "automation not found", "")
		return
	}

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	if err := permissions.CanEditAutomation(h.api, userID, existing); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, userID, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	if err := h.store.Delete(id); err != nil {
		h.api.LogError("Failed to delete automation", "user_id", userID, "automation_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to delete automation", err.Error())
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
