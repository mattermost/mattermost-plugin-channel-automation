package action

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/mattermost/mattermost-plugin-agents/public/bridgeclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingPluginAPI struct {
	lastRequest *http.Request
	response    *http.Response
}

func (r *recordingPluginAPI) PluginHTTP(req *http.Request) *http.Response {
	r.lastRequest = req
	return r.response
}

func TestAgentBridgeClient_AgentCompletionWithAgentSystemPromptIncludesFlag(t *testing.T) {
	api := &recordingPluginAPI{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(`{"completion":"done"}`)),
		},
	}
	client := NewAgentBridgeClient(api)

	completion, err := client.AgentCompletionWithAgentSystemPrompt("abcdefghijklmnopqrstuvwxyz", bridgeclient.CompletionRequest{
		Posts:        []bridgeclient.Post{{Role: "user", Message: "hello"}},
		UserID:       "user1",
		ChannelID:    "channel1",
		AllowedTools: []string{"search_posts"},
		ToolHooks: map[string]bridgeclient.ToolHookConfig{
			"search_posts": {BeforeCallback: "/hooks/tools/auto/action/before"},
		},
	}, true)
	require.NoError(t, err)
	assert.Equal(t, "done", completion)

	require.NotNil(t, api.lastRequest)
	assert.Equal(t, http.MethodPost, api.lastRequest.Method)
	assert.Equal(t, "/mattermost-ai/bridge/v1/completion/agent/abcdefghijklmnopqrstuvwxyz/nostream", api.lastRequest.URL.Path)
	assert.Equal(t, "application/json", api.lastRequest.Header.Get("Content-Type"))

	body, err := io.ReadAll(api.lastRequest.Body)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, true, payload["use_agent_system_prompt"])
	assert.Equal(t, "user1", payload["user_id"])
	assert.Equal(t, "channel1", payload["channel_id"])
	assert.Equal(t, []any{"search_posts"}, payload["allowed_tools"])
	require.Len(t, payload["posts"], 1)

	toolHooks, ok := payload["tool_hooks"].(map[string]any)
	require.True(t, ok)
	searchPostsHook, ok := toolHooks["search_posts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/hooks/tools/auto/action/before", searchPostsHook["before_callback"])
}
