package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildAutomationInstructionsResponse(t *testing.T) {
	t.Run("no URL returns base text only", func(t *testing.T) {
		out := buildAutomationInstructionsResponse(&configuration{})
		assert.Contains(t, out.Instructions, "Channel automations are trigger-action workflows")
		assert.NotContains(t, out.Instructions, "refer the user to:")
	})

	t.Run("URL appends paragraph to instructions", func(t *testing.T) {
		out := buildAutomationInstructionsResponse(&configuration{
			AutomationInstructionsURL: "https://docs.example.com/automations",
		})
		assert.Contains(t, out.Instructions, "refer the user to: https://docs.example.com/automations")
	})

	t.Run("whitespace-only URL is ignored", func(t *testing.T) {
		out := buildAutomationInstructionsResponse(&configuration{
			AutomationInstructionsURL: "   \t  ",
		})
		assert.NotContains(t, out.Instructions, "refer the user to:")
	})

	t.Run("nil config returns base text only", func(t *testing.T) {
		out := buildAutomationInstructionsResponse(nil)
		assert.Contains(t, out.Instructions, "Channel automations are trigger-action workflows")
		assert.NotContains(t, out.Instructions, "refer the user to:")
	})

	t.Run("documents the ai_prompt request_as field", func(t *testing.T) {
		out := buildAutomationInstructionsResponse(nil)
		// Field is named, both bounded enum values are listed, and the
		// permission-inheritance caveat for "creator" in shared channels
		// is preserved so agents surface it in the user-confirmation
		// summary instead of silently picking "creator".
		assert.Contains(t, out.Instructions, "request_as")
		assert.Contains(t, out.Instructions, `"triggerer"`)
		assert.Contains(t, out.Instructions, `"creator"`)
		assert.Contains(t, out.Instructions, "creator's permissions")
	})
}

func TestHandleGetAutomationInstructions(t *testing.T) {
	userID := "user1id26charslongabcdef"

	t.Run("missing Mattermost-User-ID returns 401", func(t *testing.T) {
		api := &plugintest.API{}
		api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()

		p := &Plugin{}
		p.SetAPI(api)
		p.setConfiguration(&configuration{})

		router := mux.NewRouter()
		router.Use(p.MattermostAuthorizationRequired)
		router.HandleFunc("/api/v1/automation-instructions", p.handleGetAutomationInstructions).Methods(http.MethodGet)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/automation-instructions", nil)
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("authenticated returns JSON with instructions", func(t *testing.T) {
		api := &plugintest.API{}

		p := &Plugin{}
		p.SetAPI(api)
		p.setConfiguration(&configuration{
			AutomationInstructionsURL: "https://help.example.com/channel-automation",
		})

		router := mux.NewRouter()
		router.Use(p.MattermostAuthorizationRequired)
		router.HandleFunc("/api/v1/automation-instructions", p.handleGetAutomationInstructions).Methods(http.MethodGet)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/automation-instructions", nil)
		r.Header.Set("Mattermost-User-ID", userID)
		router.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var body automationInstructionsResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Contains(t, body.Instructions, "Channel automations are trigger-action workflows")
		assert.Contains(t, body.Instructions, "https://help.example.com/channel-automation")
	})
}
