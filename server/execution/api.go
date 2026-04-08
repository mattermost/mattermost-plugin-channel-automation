package execution

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/httputil"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/permissions"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// APIHandler provides HTTP handlers for execution history.
type APIHandler struct {
	store           model.ExecutionStore
	automationStore model.Store
	api             plugin.API
}

// NewAPIHandler creates a new execution API handler.
func NewAPIHandler(store model.ExecutionStore, automationStore model.Store, api plugin.API) *APIHandler {
	return &APIHandler{store: store, automationStore: automationStore, api: api}
}

// RegisterRoutes registers the execution history routes on the given router.
func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/automations/{automation_id}/executions", h.handleListByAutomation).Methods(http.MethodGet)
	r.HandleFunc("/executions/{id}", h.handleGet).Methods(http.MethodGet)
	r.HandleFunc("/executions", h.handleListRecent).Methods(http.MethodGet)
}

func (h *APIHandler) handleListByAutomation(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	automationID := mux.Vars(r)["automation_id"]

	// Check the user has permission to view this automation.
	a, err := h.automationStore.Get(automationID)
	if err != nil {
		h.api.LogError("Failed to get automation for execution list", "user_id", userID, "automation_id", automationID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get automation", "")
		return
	}
	if a == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "automation not found", "")
		return
	}
	if permErr := permissions.CheckAutomationPermissions(h.api, userID, a); permErr != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, permErr, userID, automationID)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	limit := parseLimit(r)
	records, err := h.store.ListByAutomation(automationID, limit)
	if err != nil {
		h.api.LogError("Failed to list executions", "user_id", userID, "automation_id", automationID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to list executions", "")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(records); err != nil {
		h.api.LogError("Failed to encode response", "error", err.Error())
	}
}

func (h *APIHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	id := mux.Vars(r)["id"]

	record, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get execution record", "user_id", userID, "execution_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get execution", "")
		return
	}
	if record == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "execution not found", "")
		return
	}

	// Check the user has permission to view the parent automation.
	a, err := h.automationStore.Get(record.AutomationID)
	if err != nil {
		h.api.LogError("Failed to get automation for execution", "user_id", userID, "execution_id", id, "automation_id", record.AutomationID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get automation", "")
		return
	}
	// If the automation was deleted, only system admins can view.
	if a == nil {
		if !h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
			h.api.LogWarn("Permission denied for execution (deleted automation)", "user_id", userID, "execution_id", id, "automation_id", record.AutomationID)
			httputil.WriteErrorJSON(w, http.StatusForbidden, "forbidden", "")
			return
		}
	} else if err := permissions.CheckAutomationPermissions(h.api, userID, a); err != nil {
		msg, code, detail := permissions.HandlePermissionError(h.api, err, userID, record.AutomationID)
		httputil.WriteErrorJSON(w, code, msg, detail)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(record); err != nil {
		h.api.LogError("Failed to encode response", "error", err.Error())
	}
}

func (h *APIHandler) handleListRecent(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header", "")
		return
	}

	// Only system admins can list all executions.
	if !h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
		h.api.LogWarn("Permission denied for recent executions list", "user_id", userID)
		httputil.WriteErrorJSON(w, http.StatusForbidden, "forbidden", "")
		return
	}

	limit := parseLimit(r)
	records, err := h.store.ListRecent(limit)
	if err != nil {
		h.api.LogError("Failed to list recent executions", "user_id", userID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to list executions", "")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(records); err != nil {
		h.api.LogError("Failed to encode response", "error", err.Error())
	}
}

func parseLimit(r *http.Request) int {
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			if n > maxLimit {
				return maxLimit
			}
			return n
		}
	}
	return defaultLimit
}
