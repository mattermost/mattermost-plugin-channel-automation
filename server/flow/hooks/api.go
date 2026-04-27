// Package hooks implements HTTP callbacks for MCP tool before hooks when
// ai_prompt actions use channel guardrails.
package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost-plugin-agents/public/mcptool"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const (
	maxHookBodySize = 1 << 20 // 1 MB

	// headerMattermostUserID is the request header Mattermost sets to the
	// authenticated session user ID for plugin HTTP requests.
	headerMattermostUserID = "Mattermost-User-ID"
)

// APIHandler serves tool hook callbacks for channel guardrails.
type APIHandler struct {
	store model.Store
	api   plugin.API
	// channelTeamCache memoizes channel_id -> team_id (or "" for DM/GM).
	// Channel -> team is immutable in Mattermost so entries never expire.
	// On plugin restart the cache rebuilds on demand.
	channelTeamCache sync.Map
}

// HookCtx carries guardrail state and dependencies for a single hook invocation.
type HookCtx struct {
	Guardrails *model.Guardrails
	AllowedCh  map[string]struct{}
	// AllowedTeams is the set of team IDs the LLM is allowed to query through
	// team tools. It contains the team of every guardrail channel plus the
	// flow's trigger team (when resolvable).
	AllowedTeams map[string]struct{}
	API          plugin.API
	UserID       string
}

// NewAPIHandler constructs a hooks API handler.
func NewAPIHandler(store model.Store, api plugin.API) *APIHandler {
	return &APIHandler{store: store, api: api}
}

// HookURL returns the plugin-relative before-callback URL for a tool hook on
// the given flow/action. Centralized so the route registration and the URL
// emitted to the bridge cannot drift apart.
func HookURL(flowID, actionID string) string {
	return fmt.Sprintf("/api/v1/hooks/tools/%s/%s/before", flowID, actionID)
}

// RegisterRoutes registers POST /hooks/tools/{flow_id}/{action_id}/before.
func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/hooks/tools/{flow_id}/{action_id}/before", h.handleBefore).Methods(http.MethodPost)
}

// resolveChannelTeam returns the team ID for the given channel, consulting
// the cache first and falling back to plugin.API.GetChannel. The result is
// memoized including the empty-team case (DM/GM channels).
func (h *APIHandler) resolveChannelTeam(channelID string) (string, error) {
	if channelID == "" {
		return "", nil
	}
	if v, ok := h.channelTeamCache.Load(channelID); ok {
		return v.(string), nil
	}
	ch, appErr := h.api.GetChannel(channelID)
	if appErr != nil {
		return "", fmt.Errorf("resolve channel %q team: %s", channelID, appErr.Error())
	}
	if ch == nil {
		return "", fmt.Errorf("resolve channel %q team: channel not found", channelID)
	}
	h.channelTeamCache.Store(channelID, ch.TeamId)
	return ch.TeamId, nil
}

// allowedSetsForGuardrails computes the AllowedCh and AllowedTeams sets for a
// guardrail block, resolving channel -> team for any entry not already
// carrying a TeamID. Channels whose team cannot be resolved are still added
// to AllowedCh (so channel tools work) but contribute nothing to
// AllowedTeams. The resolution is logged at debug for traceability.
func (h *APIHandler) allowedSetsForGuardrails(gr *model.Guardrails, flowID, actionID string) (map[string]struct{}, map[string]struct{}) {
	chs := make(map[string]struct{}, len(gr.Channels))
	teams := make(map[string]struct{}, len(gr.Channels))
	for i := range gr.Channels {
		c := &gr.Channels[i]
		cid := strings.TrimSpace(c.ChannelID)
		if cid == "" {
			continue
		}
		chs[cid] = struct{}{}
		if c.TeamID == "" {
			tid, err := h.resolveChannelTeam(cid)
			if err != nil {
				h.api.LogDebug("hooks: failed to resolve guardrail channel team",
					"flow_id", flowID,
					"action_id", actionID,
					"channel_id", cid,
					"error", err.Error(),
				)
				continue
			}
			c.TeamID = tid
		}
		if c.TeamID != "" {
			teams[c.TeamID] = struct{}{}
		}
	}
	return chs, teams
}

// authorizeFlowCreator returns true if the request is authenticated as the
// flow's creator. Otherwise it writes a 403 JSON error and returns false.
// The flow must have a non-empty CreatedBy; flows missing a creator are
// treated as unauthorized to prevent accidental open access.
func (h *APIHandler) authorizeFlowCreator(w http.ResponseWriter, r *http.Request, f *model.Flow, errResp any) bool {
	callerID := r.Header.Get(headerMattermostUserID)
	if callerID == "" || f.CreatedBy == "" || callerID != f.CreatedBy {
		h.api.LogWarn("hooks: unauthorized hook caller",
			"flow_id", f.ID,
			"caller_user_id", callerID,
			"creator_user_id", f.CreatedBy,
			"path", r.URL.Path,
		)
		writeJSON(w, http.StatusForbidden, errResp)
		return false
	}
	return true
}

// loadGuardrailFlow loads the flow and the guardrails for the given action when
// channel guardrails are configured.
func (h *APIHandler) loadGuardrailFlow(flowID, actionID string) (*model.Flow, *model.Guardrails, bool) {
	f, err := h.store.Get(flowID)
	if err != nil || f == nil {
		return nil, nil, false
	}
	gr := f.GuardrailsForAction(actionID)
	if gr == nil || len(gr.Channels) == 0 {
		return nil, nil, false
	}
	return f, gr, true
}

// flowAnchorTeamID returns the Mattermost team ID the flow is anchored to:
// the team_id of a channel_created or user_joined_team trigger, or the team
// of the trigger channel for channel-scoped triggers. It is best-effort:
// benign "no anchor team" cases (no trigger channel, channel_created/user_joined_team
// without team_id, DM/GM trigger channel) return ("", nil). Only unexpected
// failures (nil flow, GetChannel error) are returned as errors so callers can
// log them.
func flowAnchorTeamID(api plugin.API, f *model.Flow) (string, error) {
	if f == nil {
		return "", fmt.Errorf("flow not loaded")
	}
	if f.Trigger.ChannelCreated != nil {
		return f.Trigger.ChannelCreated.TeamID, nil
	}
	if f.Trigger.UserJoinedTeam != nil {
		return f.Trigger.UserJoinedTeam.TeamID, nil
	}
	chID := f.TriggerChannelID()
	if chID == "" {
		return "", nil
	}
	ch, appErr := api.GetChannel(chID)
	if appErr != nil {
		return "", fmt.Errorf("get channel %q: %w", chID, appErr)
	}
	if ch == nil {
		return "", nil
	}
	return ch.TeamId, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSONBody(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxHookBodySize)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing data in JSON body")
	}
	return nil
}

func (h *APIHandler) handleBefore(w http.ResponseWriter, r *http.Request) {
	var req mcptool.BeforeHookRequest
	if err := readJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, mcptool.BeforeHookResponse{Error: "invalid request body"})
		return
	}
	vars := mux.Vars(r)
	flowID := vars["flow_id"]
	actionID := vars["action_id"]

	f, gr, ok := h.loadGuardrailFlow(flowID, actionID)
	if !ok {
		writeJSON(w, http.StatusOK, mcptool.BeforeHookResponse{Error: "guardrails not found"})
		return
	}

	if !h.authorizeFlowCreator(w, r, f, mcptool.BeforeHookResponse{Error: "forbidden: only the automation creator may invoke hooks"}) {
		return
	}

	if req.ToolName == "" {
		writeJSON(w, http.StatusOK, mcptool.BeforeHookResponse{Error: "tool_name is required"})
		return
	}
	entry, catOK := LookupMattermostMCPTool(req.ToolName)
	if !catOK {
		writeJSON(w, http.StatusOK, mcptool.BeforeHookResponse{
			Error: fmt.Sprintf("tool %q is not a known Mattermost MCP server tool; channel guardrails reject unrecognized tools",
				req.ToolName),
		})
		return
	}
	if entry.Before == nil {
		writeJSON(w, http.StatusOK, mcptool.BeforeHookResponse{
			Error: fmt.Sprintf("tool %q is not supported with channel guardrails (Mattermost MCP tools must be explicitly allowlisted for guardrails)",
				req.ToolName),
		})
		return
	}

	var argsMap map[string]any
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &argsMap); err != nil {
			writeJSON(w, http.StatusOK, mcptool.BeforeHookResponse{Error: "invalid args JSON in hook request"})
			return
		}
	}

	allowedCh, allowedTeams := h.allowedSetsForGuardrails(gr, flowID, actionID)
	if expTeam, err := flowAnchorTeamID(h.api, f); err != nil {
		h.api.LogDebug("hooks: failed to resolve flow anchor team",
			"flow_id", flowID, "action_id", actionID, "error", err.Error())
	} else if expTeam != "" {
		allowedTeams[expTeam] = struct{}{}
	}

	ctx := HookCtx{
		Guardrails:   gr,
		AllowedCh:    allowedCh,
		AllowedTeams: allowedTeams,
		API:          h.api,
		UserID:       req.UserID,
	}
	if err := entry.Before(ctx, argsMap); err != nil {
		writeJSON(w, http.StatusOK, mcptool.BeforeHookResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, mcptool.BeforeHookResponse{})
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}
