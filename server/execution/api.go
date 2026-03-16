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
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// APIHandler provides HTTP handlers for execution history.
type APIHandler struct {
	store     model.ExecutionStore
	flowStore model.Store
	api       plugin.API
}

// NewAPIHandler creates a new execution API handler.
func NewAPIHandler(store model.ExecutionStore, flowStore model.Store, api plugin.API) *APIHandler {
	return &APIHandler{store: store, flowStore: flowStore, api: api}
}

// RegisterRoutes registers the execution history routes on the given router.
func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/flows/{flow_id}/executions", h.handleListByFlow).Methods(http.MethodGet)
	r.HandleFunc("/executions/{id}", h.handleGet).Methods(http.MethodGet)
	r.HandleFunc("/executions", h.handleListRecent).Methods(http.MethodGet)
}

func (h *APIHandler) handleListByFlow(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header")
		return
	}

	flowID := mux.Vars(r)["flow_id"]

	// Check the user has permission to view this flow.
	f, err := h.flowStore.Get(flowID)
	if err != nil {
		h.api.LogError("Failed to get flow for execution list", "user_id", userID, "flow_id", flowID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get flow", err.Error())
		return
	}
	if f == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "flow not found")
		return
	}
	if !h.checkPermission(userID, f) {
		h.api.LogWarn("Permission denied for execution list", "user_id", userID, "flow_id", flowID)
		httputil.WriteErrorJSON(w, http.StatusForbidden, "forbidden")
		return
	}

	limit := parseLimit(r)
	records, err := h.store.ListByFlow(flowID, limit)
	if err != nil {
		h.api.LogError("Failed to list executions", "user_id", userID, "flow_id", flowID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to list executions", err.Error())
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
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header")
		return
	}

	id := mux.Vars(r)["id"]
	record, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get execution record", "user_id", userID, "execution_id", id, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get execution", err.Error())
		return
	}
	if record == nil {
		httputil.WriteErrorJSON(w, http.StatusNotFound, "execution not found")
		return
	}

	// Check the user has permission to view the parent flow.
	f, err := h.flowStore.Get(record.FlowID)
	if err != nil {
		h.api.LogError("Failed to get flow for execution", "user_id", userID, "execution_id", id, "flow_id", record.FlowID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get flow", err.Error())
		return
	}
	// If the flow was deleted, only system admins can view.
	if f == nil {
		if !h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
			h.api.LogWarn("Permission denied for execution (deleted flow)", "user_id", userID, "execution_id", id, "flow_id", record.FlowID)
			httputil.WriteErrorJSON(w, http.StatusForbidden, "forbidden")
			return
		}
	} else if !h.checkPermission(userID, f) {
		h.api.LogWarn("Permission denied for execution", "user_id", userID, "execution_id", id, "flow_id", record.FlowID)
		httputil.WriteErrorJSON(w, http.StatusForbidden, "forbidden")
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
		httputil.WriteErrorJSON(w, http.StatusUnauthorized, "missing Mattermost-User-ID header")
		return
	}

	// Only system admins can list all executions.
	if !h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
		h.api.LogWarn("Permission denied for recent executions list", "user_id", userID)
		httputil.WriteErrorJSON(w, http.StatusForbidden, "forbidden")
		return
	}

	limit := parseLimit(r)
	records, err := h.store.ListRecent(limit)
	if err != nil {
		h.api.LogError("Failed to list recent executions", "user_id", userID, "error", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to list executions", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(records); err != nil {
		h.api.LogError("Failed to encode response", "error", err.Error())
	}
}

// checkPermission returns true if the user is a system admin or has
// channel admin permissions on every channel referenced by the flow.
func (h *APIHandler) checkPermission(userID string, f *model.Flow) bool {
	if h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
		return true
	}
	for _, chID := range model.CollectChannelIDs(f) {
		member, appErr := h.api.GetChannelMember(chID, userID)
		if appErr != nil || !member.SchemeAdmin {
			return false
		}
	}
	return true
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
