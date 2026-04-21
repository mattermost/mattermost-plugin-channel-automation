// Package hooks implements HTTP callbacks for MCP tool before/after hooks when
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

const maxHookBodySize = 1 << 20 // 1 MB

// APIHandler serves tool hook callbacks for channel guardrails.
type APIHandler struct {
	store model.Store
	api   plugin.API
	// channelTeamCache memoizes channel_id -> team_id (or "" for DM/GM).
	// Channel -> team is immutable in Mattermost so entries never expire.
	// On plugin restart the cache rebuilds on demand.
	channelTeamCache sync.Map
}

// NewAPIHandler constructs a hooks API handler.
func NewAPIHandler(store model.Store, api plugin.API) *APIHandler {
	return &APIHandler{store: store, api: api}
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

// RegisterRoutes registers POST /hooks/tools/{flow_id}/{action_id}/before|after.
func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/hooks/tools/{flow_id}/{action_id}/before", h.handleBefore).Methods(http.MethodPost)
	r.HandleFunc("/hooks/tools/{flow_id}/{action_id}/after", h.handleAfter).Methods(http.MethodPost)
}

// loadGuardrailFlow loads the flow and the guardrails for the given action when
// channel guardrails are configured.
func (h *APIHandler) loadGuardrailFlow(flowID, actionID string) (*model.Flow, *model.Guardrails, bool) {
	f, err := h.store.Get(flowID)
	if err != nil || f == nil {
		return nil, nil, false
	}
	for i := range f.Actions {
		a := &f.Actions[i]
		if a.ID != actionID || a.AIPrompt == nil || a.AIPrompt.Guardrails == nil {
			continue
		}
		gr := a.AIPrompt.Guardrails
		if len(gr.Channels) == 0 {
			return nil, nil, false
		}
		return f, gr, true
	}
	return nil, nil, false
}

// expectedTeamID returns the Mattermost team ID the flow is anchored to: the
// channel_created trigger's team_id, or the team of the trigger channel.
func expectedTeamID(api plugin.API, f *model.Flow) (string, error) {
	if f == nil {
		return "", fmt.Errorf("cannot determine automation team: flow not loaded")
	}
	if f.Trigger.ChannelCreated != nil {
		if f.Trigger.ChannelCreated.TeamID == "" {
			return "", fmt.Errorf("cannot determine automation team: channel_created trigger has no team_id")
		}
		return f.Trigger.ChannelCreated.TeamID, nil
	}
	chID := f.TriggerChannelID()
	if chID == "" {
		return "", fmt.Errorf("cannot determine automation team: flow has no trigger channel")
	}
	ch, appErr := api.GetChannel(chID)
	if appErr != nil {
		return "", fmt.Errorf("cannot determine automation team: %w", appErr)
	}
	if ch == nil || ch.TeamId == "" {
		return "", fmt.Errorf("cannot determine automation team: channel missing team id")
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

	expTeam, teamErr := expectedTeamID(h.api, f)
	teamFromFlowErr := ""
	if teamErr != nil {
		teamFromFlowErr = teamErr.Error()
	}

	allowedCh, allowedTeams := h.allowedSetsForGuardrails(gr, flowID, actionID)
	if expTeam != "" {
		allowedTeams[expTeam] = struct{}{}
	}

	ctx := HookCtx{
		Guardrails:      gr,
		AllowedCh:       allowedCh,
		AllowedTeams:    allowedTeams,
		API:             h.api,
		UserID:          req.UserID,
		ExpectedTeamID:  expTeam,
		TeamFromFlowErr: teamFromFlowErr,
	}
	if err := entry.Before(ctx, req.Args); err != nil {
		writeJSON(w, http.StatusOK, mcptool.BeforeHookResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, mcptool.BeforeHookResponse{})
}

func (h *APIHandler) handleAfter(w http.ResponseWriter, r *http.Request) {
	var req mcptool.AfterHookRequest
	if err := readJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, mcptool.AfterHookResponse{Error: "invalid request body"})
		return
	}
	vars := mux.Vars(r)
	flowID := vars["flow_id"]
	actionID := vars["action_id"]

	f, gr, ok := h.loadGuardrailFlow(flowID, actionID)
	if !ok {
		writeJSON(w, http.StatusOK, mcptool.AfterHookResponse{Error: "guardrails not found"})
		return
	}

	if req.Error != "" {
		writeJSON(w, http.StatusOK, mcptool.AfterHookResponse{})
		return
	}

	if req.ToolName == "" {
		writeJSON(w, http.StatusOK, mcptool.AfterHookResponse{Error: "tool_name is required"})
		return
	}
	entry, catOK := LookupMattermostMCPTool(req.ToolName)
	if !catOK {
		writeJSON(w, http.StatusOK, mcptool.AfterHookResponse{
			Error: fmt.Sprintf("tool %q is not a known Mattermost MCP server tool; channel guardrails reject unrecognized tools",
				req.ToolName),
		})
		return
	}
	if entry.After == nil {
		writeJSON(w, http.StatusOK, mcptool.AfterHookResponse{
			Error: fmt.Sprintf("tool %q is not supported with channel guardrails (Mattermost MCP tools must be explicitly allowlisted for guardrails)",
				req.ToolName),
		})
		return
	}

	expTeam, teamErr := expectedTeamID(h.api, f)
	teamFromFlowErr := ""
	if teamErr != nil {
		teamFromFlowErr = teamErr.Error()
	}

	allowedCh, allowedTeams := h.allowedSetsForGuardrails(gr, flowID, actionID)
	if expTeam != "" {
		allowedTeams[expTeam] = struct{}{}
	}

	ctx := HookCtx{
		Guardrails:      gr,
		AllowedCh:       allowedCh,
		AllowedTeams:    allowedTeams,
		API:             h.api,
		UserID:          req.UserID,
		ExpectedTeamID:  expTeam,
		TeamFromFlowErr: teamFromFlowErr,
	}
	out, err := entry.After(ctx, req.Output)
	if err != nil {
		writeJSON(w, http.StatusOK, mcptool.AfterHookResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, mcptool.AfterHookResponse{Output: out})
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
	// ExpectedTeamID is the team this automation is anchored to (from the flow).
	ExpectedTeamID string
	// TeamFromFlowErr is non-empty when ExpectedTeamID could not be resolved; get_team_* hooks must fail.
	TeamFromFlowErr string
}

// maxLoggedPayload caps the size of args/output payloads emitted to debug logs to
// avoid flooding logs with very large MCP tool payloads.
const maxLoggedPayload = 4096

func argsForLog(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		return fmt.Sprintf("<unmarshalable args: %v>", err)
	}
	return truncateForLog(string(b))
}

func outputForLog(output json.RawMessage) string {
	if len(output) == 0 {
		return ""
	}
	return truncateForLog(string(output))
}

func truncateForLog(s string) string {
	if len(s) <= maxLoggedPayload {
		return s
	}
	return s[:maxLoggedPayload] + fmt.Sprintf("...(truncated, %d bytes total)", len(s))
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
