package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/execution"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow"
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
	flowAPI := flow.NewAPIHandler(p.flowStore, p.historyStore, p.API, p.scheduleManager, &configProvider{getConfig: p.getConfiguration})
	flowAPI.RegisterRoutes(apiRouter)

	execAPI := execution.NewAPIHandler(p.historyStore, p.flowStore, p.API)
	execAPI.RegisterRoutes(apiRouter)

	apiRouter.HandleFunc("/agents/{agent_id}/tools", p.handleGetAgentTools).Methods(http.MethodGet)

	return router
}

// handleGetAgentTools proxies a request to the AI plugin bridge to retrieve the
// tools available for a specific agent.
func (p *Plugin) handleGetAgentTools(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	agentID := mux.Vars(r)["agent_id"]

	if p.bridgeClient == nil {
		p.API.LogWarn("AI plugin bridge not available", "user_id", userID, "agent_id", agentID)
		http.Error(w, "AI plugin bridge not available", http.StatusServiceUnavailable)
		return
	}

	tools, err := p.bridgeClient.GetAgentTools(agentID, userID)
	if err != nil {
		p.API.LogError("Failed to get agent tools", "user_id", userID, "agent_id", agentID, "err", err.Error())
		http.Error(w, "failed to get agent tools", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tools); err != nil {
		p.API.LogError("Failed to encode response", "user_id", userID, "agent_id", agentID, "error", err.Error())
	}
}

// ServeHTTP handles HTTP requests for the plugin API.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}
