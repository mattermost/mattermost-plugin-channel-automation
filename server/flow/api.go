package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/hooks"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/httputil"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/permissions"
)

const maxRequestBodySize = 1 << 20 // 1 MB

// APIHandler provides HTTP handlers for flow CRUD operations.
type APIHandler struct {
	store           model.Store
	historyStore    model.ExecutionStore
	api             plugin.API
	registry        *Registry
	scheduleManager *ScheduleManager
	config          model.Configuration
	bridge          hooks.AgentToolsLister
}

// NewAPIHandler creates a new flow API handler. bridge may be nil in tests
// that do not exercise allowed_tools validation.
func NewAPIHandler(store model.Store, historyStore model.ExecutionStore, api plugin.API, registry *Registry, scheduleManager *ScheduleManager, config model.Configuration, bridge hooks.AgentToolsLister) *APIHandler {
	return &APIHandler{store: store, historyStore: historyStore, api: api, registry: registry, scheduleManager: scheduleManager, config: config, bridge: bridge}
}

// RegisterRoutes registers the flow CRUD routes on the given router.
func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/flows", h.handleListFlows).Methods(http.MethodGet)
	r.HandleFunc("/flows", h.handleCreateFlow).Methods(http.MethodPost)
	r.HandleFunc("/flows/{id}", h.handleGetFlow).Methods(http.MethodGet)
	r.HandleFunc("/flows/{id}", h.handleUpdateFlow).Methods(http.MethodPut)
	r.HandleFunc("/flows/{id}", h.handleDeleteFlow).Methods(http.MethodDelete)
}

// channelFlowLimit returns the configured per-channel flow limit, or 0
// when no limit is configured.
func (h *APIHandler) channelFlowLimit() int {
	if h.config == nil {
		return 0
	}
	return h.config.MaxFlowsPerChannel()
}

func (h *APIHandler) handleListFlows(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
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
		h.api.LogError("Failed to list flows", "user_id", userID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to list flows", err.Error())
		return
	}

	// Filter flows to only those the user has permission to view.
	isAdmin := h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem)
	if !isAdmin {
		visible := make([]*model.Flow, 0, len(flows))
		for _, f := range flows {
			if permissions.CheckFlowPermissions(h.api, userID, f) == nil {
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

	if err := permissions.CheckFlowPermissions(h.api, f.CreatedBy, &f); err != nil {
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
	// checks so we never call the bridge for flows the user cannot manage.
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

	limit := h.channelFlowLimit()
	if err := h.store.SaveWithChannelLimit(&f, limit, ""); err != nil {
		if errors.Is(err, ErrChannelFlowLimitExceeded) {
			msg := fmt.Sprintf("channel has reached the maximum of %d flow(s)", limit)
			httputil.WriteErrorJSON(w, http.StatusConflict, msg, "")
			return
		}
		h.api.LogError("Failed to create flow", "user_id", f.CreatedBy, "flow_id", f.ID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to create flow", err.Error())
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
		h.api.LogError("Failed to get flow", "flow_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get flow", err.Error())
		return
	}
	if f == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "flow not found", "")
		return
	}

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	if err := permissions.CheckFlowPermissions(h.api, userID, f); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, userID, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
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
		h.api.LogError("Failed to get flow for update", "flow_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get flow", err.Error())
		return
	}
	if existing == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "flow not found", "")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var f model.Flow
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

	// Only the creator (or a sysadmin acting on their behalf) may edit a flow.
	// This is the security boundary that lets the downstream creator-anchored
	// checks below validate against existing.CreatedBy without enabling
	// privilege escalation by a non-creator editor.
	if err := permissions.CanEditFlow(h.api, userID, existing); err != nil {
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

	// New flow configuration must remain admissible for the creator (covers
	// the sysadmin-edit case: catches references the creator cannot manage).
	if err := permissions.CheckFlowPermissions(h.api, existing.CreatedBy, &f); err != nil {
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
	// checks so we never call the bridge for inadmissible flows.
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

	limit := h.channelFlowLimit()
	if err := h.store.SaveWithChannelLimit(&f, limit, f.ID); err != nil {
		if errors.Is(err, ErrChannelFlowLimitExceeded) {
			msg := fmt.Sprintf("channel has reached the maximum of %d flow(s)", limit)
			httputil.WriteErrorJSON(w, http.StatusConflict, msg, "")
			return
		}
		h.api.LogError("Failed to update flow", "user_id", userID, "flow_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to update flow", err.Error())
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
		h.api.LogError("Failed to get flow for delete", "flow_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get flow", err.Error())
		return
	}
	if existing == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "flow not found", "")
		return
	}

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	if err := permissions.CanEditFlow(h.api, userID, existing); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, userID, id)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	if err := h.store.Delete(id); err != nil {
		h.api.LogError("Failed to delete flow", "user_id", userID, "flow_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to delete flow", err.Error())
		return
	}

	if h.scheduleManager != nil {
		h.scheduleManager.RemoveFlow(id)
	}

	if h.historyStore != nil {
		if err := h.historyStore.PurgeFlow(id); err != nil {
			h.api.LogError("Failed to purge execution history", "user_id", userID, "flow_id", id, "error", err.Error())
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
