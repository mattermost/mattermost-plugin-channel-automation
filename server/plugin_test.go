package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost-plugin-ai/public/bridgeclient"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestServeHTTP(t *testing.T) {
	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		plugin := Plugin{}
		router := mux.NewRouter()
		router.Use(plugin.MattermostAuthorizationRequired)
		router.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		plugin.router = router

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test", nil)

		plugin.ServeHTTP(nil, w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("unknown route returns 404", func(t *testing.T) {
		plugin := Plugin{}
		plugin.router = mux.NewRouter()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)

		plugin.ServeHTTP(nil, w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandleGetAgentTools(t *testing.T) {
	// Use valid 26-char Mattermost IDs to pass bridge client validation.
	agentID := mmmodel.NewId()
	userID := mmmodel.NewId()

	t.Run("nil bridge client returns 503", func(t *testing.T) {
		p := &Plugin{}

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/agents/{agent_id}/tools", p.handleGetAgentTools).Methods(http.MethodGet)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/tools", nil)
		r.Header.Set("Mattermost-User-ID", userID)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Contains(t, w.Body.String(), "AI plugin bridge not available")
	})

	t.Run("successful proxy returns tools JSON", func(t *testing.T) {
		toolsResp := bridgeclient.AgentToolsResponse{
			Tools: []bridgeclient.BridgeToolInfo{
				{Name: "search", Description: "Search things"},
			},
		}
		respBody, _ := json.Marshal(toolsResp)

		api := &plugintest.API{}
		api.On("PluginHTTP", mock.MatchedBy(func(req *http.Request) bool {
			return strings.Contains(req.URL.Path, "/bridge/v1/agents/")
		})).Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(respBody))),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		})

		p := &Plugin{}
		p.SetAPI(api)
		p.bridgeClient = bridgeclient.NewClient(api)

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/agents/{agent_id}/tools", p.handleGetAgentTools).Methods(http.MethodGet)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/tools", nil)
		r.Header.Set("Mattermost-User-ID", userID)

		router.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), `"name":"search"`)
	})

	t.Run("bridge client error returns 502", func(t *testing.T) {
		api := &plugintest.API{}
		api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
		api.On("PluginHTTP", mock.Anything).Return(&http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
		})

		p := &Plugin{}
		p.SetAPI(api)
		p.bridgeClient = bridgeclient.NewClient(api)

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/agents/{agent_id}/tools", p.handleGetAgentTools).Methods(http.MethodGet)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/tools", nil)
		r.Header.Set("Mattermost-User-ID", userID)

		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadGateway, w.Code)
	})
}
