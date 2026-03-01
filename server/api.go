package main

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow"
)

// initRouter initializes the HTTP router for the plugin.
func (p *Plugin) initRouter() *mux.Router {
	router := mux.NewRouter()

	router.Use(p.MattermostAuthorizationRequired)

	// Management plugin API
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.Use(p.SystemAdminRequired)
	flowAPI := flow.NewAPIHandler(p.flowStore, p.API)
	flowAPI.RegisterRoutes(apiRouter)

	return router
}

// ServeHTTP handles HTTP requests for the plugin API.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}
