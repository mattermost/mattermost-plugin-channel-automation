package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/execution"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/httputil"
)

// configProvider adapts the plugin's unexported configuration to the
// flow.Configuration interface. It calls getConfig on each access so
// it always reflects the current configuration.
type configProvider struct {
	getConfig func() *configuration
}

func (c *configProvider) MaxFlowsPerChannel() int {
	return c.getConfig().MaxFlowsPerChannelLimit
}

// initRouter initializes the HTTP router for the plugin.
func (p *Plugin) initRouter() *mux.Router {
	router := mux.NewRouter()

	router.Use(p.MattermostAuthorizationRequired)

	// Management plugin API
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	flowAPI := flow.NewAPIHandler(p.flowStore, p.historyStore, p.API, p.scheduleManager, &configProvider{getConfig: p.getConfiguration}, p.botManager)
	flowAPI.RegisterRoutes(apiRouter)

	execAPI := execution.NewAPIHandler(p.historyStore, p.flowStore, p.API)
	execAPI.RegisterRoutes(apiRouter)

	apiRouter.HandleFunc("/config", p.handleGetClientConfig).Methods(http.MethodGet)
	apiRouter.HandleFunc("/agents/{agent_id}/tools", p.handleGetAgentTools).Methods(http.MethodGet)
	apiRouter.HandleFunc("/teams/{team_id}/bot", p.handleGetTeamBot).Methods(http.MethodGet)

	return router
}

// clientConfig is the subset of configuration returned to webapp clients.
type clientConfig struct {
	EnableUI bool `json:"enable_ui"`
}

// handleGetClientConfig returns the client-relevant plugin configuration.
func (p *Plugin) handleGetClientConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := p.getConfiguration()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(clientConfig{EnableUI: cfg.EnableUI}); err != nil {
		p.API.LogError("Failed to encode client config", "error", err.Error())
	}
}

// handleGetAgentTools proxies a request to the AI plugin bridge to retrieve the
// tools available for a specific agent.
func (p *Plugin) handleGetAgentTools(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	agentID := mux.Vars(r)["agent_id"]

	if p.bridgeClient == nil {
		p.API.LogWarn("AI plugin bridge not available", "user_id", userID, "agent_id", agentID)
		httputil.WriteErrorJSON(w, http.StatusServiceUnavailable, "AI plugin bridge not available", "")
		return
	}

	tools, err := p.bridgeClient.GetAgentTools(agentID, userID)
	if err != nil {
		p.API.LogError("Failed to get agent tools", "user_id", userID, "agent_id", agentID, "err", err.Error())
		httputil.WriteErrorJSON(w, http.StatusBadGateway, "failed to get agent tools", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tools); err != nil {
		p.API.LogError("Failed to encode response", "user_id", userID, "agent_id", agentID, "error", err.Error())
	}
}

// teamBotResponse is the JSON response for the team bot endpoint.
type teamBotResponse struct {
	BotUserID string `json:"bot_user_id"`
	TeamID    string `json:"team_id"`
	Username  string `json:"username"`
}

// handleGetTeamBot returns (or creates) the automation bot for a team.
func (p *Plugin) handleGetTeamBot(w http.ResponseWriter, r *http.Request) {
	teamID := mux.Vars(r)["team_id"]

	botUserID, err := p.botManager.EnsureTeamBot(teamID)
	if err != nil {
		p.API.LogError("Failed to ensure team bot", "team_id", teamID, "err", err.Error())
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to create or retrieve team bot", err.Error())
		return
	}

	botUser, appErr := p.API.GetUser(botUserID)
	if appErr != nil {
		httputil.WriteErrorJSON(w, http.StatusInternalServerError, "failed to get bot user", appErr.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(teamBotResponse{
		BotUserID: botUserID,
		TeamID:    teamID,
		Username:  botUser.Username,
	}); err != nil {
		p.API.LogError("Failed to encode team bot response", "error", err.Error())
	}
}

// ServeHTTP handles HTTP requests for the plugin API.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}
