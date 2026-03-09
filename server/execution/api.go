package execution

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const defaultLimit = 20

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
		http.Error(w, "missing user ID", http.StatusUnauthorized)
		return
	}

	flowID := mux.Vars(r)["flow_id"]

	// Check the user has permission to view this flow.
	f, err := h.flowStore.Get(flowID)
	if err != nil {
		h.api.LogError("Failed to get flow for execution list", "error", err.Error())
		http.Error(w, "failed to get flow", http.StatusInternalServerError)
		return
	}
	if f == nil {
		http.Error(w, "flow not found", http.StatusNotFound)
		return
	}
	if !h.checkPermission(userID, f) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	limit := parseLimit(r)
	records, err := h.store.ListByFlow(flowID, limit)
	if err != nil {
		h.api.LogError("Failed to list executions", "error", err.Error())
		http.Error(w, "failed to list executions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(records)
}

func (h *APIHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, "missing user ID", http.StatusUnauthorized)
		return
	}

	id := mux.Vars(r)["id"]
	record, err := h.store.Get(id)
	if err != nil {
		h.api.LogError("Failed to get execution record", "error", err.Error())
		http.Error(w, "failed to get execution", http.StatusInternalServerError)
		return
	}
	if record == nil {
		http.Error(w, "execution not found", http.StatusNotFound)
		return
	}

	// Check the user has permission to view the parent flow.
	f, err := h.flowStore.Get(record.FlowID)
	if err != nil {
		h.api.LogError("Failed to get flow for execution", "error", err.Error())
		http.Error(w, "failed to get flow", http.StatusInternalServerError)
		return
	}
	// If the flow was deleted, only system admins can view.
	if f == nil {
		if !h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	} else if !h.checkPermission(userID, f) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(record)
}

func (h *APIHandler) handleListRecent(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, "missing user ID", http.StatusUnauthorized)
		return
	}

	// Only system admins can list all executions.
	if !h.api.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	limit := parseLimit(r)
	records, err := h.store.ListRecent(limit)
	if err != nil {
		h.api.LogError("Failed to list recent executions", "error", err.Error())
		http.Error(w, "failed to list executions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(records)
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
			return n
		}
	}
	return defaultLimit
}
