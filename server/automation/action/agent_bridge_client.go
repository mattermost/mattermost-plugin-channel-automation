package action

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mattermost/mattermost-plugin-agents/public/bridgeclient"
)

const agentsPluginID = "mattermost-ai"

// AgentBridgeClient wraps the Agents bridge client with the Channel Automation
// fields that may not yet be exposed by the upstream Go client.
type AgentBridgeClient struct {
	*bridgeclient.Client
	api bridgeclient.PluginAPI
}

// NewAgentBridgeClient creates an Agents bridge client for plugin HTTP calls.
func NewAgentBridgeClient(api bridgeclient.PluginAPI) *AgentBridgeClient {
	return &AgentBridgeClient{
		Client: bridgeclient.NewClient(api),
		api:    api,
	}
}

type completionRequestWithAgentSystemPrompt struct {
	bridgeclient.CompletionRequest
	UseAgentSystemPrompt bool `json:"use_agent_system_prompt,omitempty"`
}

// AgentCompletionWithAgentSystemPrompt sends an agent completion request and
// includes use_agent_system_prompt when the automation opts in.
func (c *AgentBridgeClient) AgentCompletionWithAgentSystemPrompt(agent string, request bridgeclient.CompletionRequest, useAgentSystemPrompt bool) (string, error) {
	if !useAgentSystemPrompt {
		return c.AgentCompletion(agent, request)
	}
	if c == nil || c.api == nil {
		return "", fmt.Errorf("agents plugin bridge client is not configured")
	}
	if err := bridgeclient.ValidateID(agent); err != nil {
		return "", fmt.Errorf("invalid agent ID: %w", err)
	}

	body, err := json.Marshal(completionRequestWithAgentSystemPrompt{
		CompletionRequest:    request,
		UseAgentSystemPrompt: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("/%s/bridge/v1/completion/agent/%s/nostream", agentsPluginID, agent), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp := c.api.PluginHTTP(req)
	if resp == nil {
		return "", fmt.Errorf("failed to make interplugin request")
	}
	if resp.Body == nil {
		return "", fmt.Errorf("failed to make interplugin request: empty response body")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", requestFailedError(resp.StatusCode, respBody)
	}

	var completionResp bridgeclient.CompletionResponse
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return completionResp.Completion, nil
}

func requestFailedError(statusCode int, responseBody []byte) error {
	var errResp bridgeclient.ErrorResponse
	if err := json.Unmarshal(responseBody, &errResp); err == nil {
		errMessage := strings.TrimSpace(errResp.Error)
		if errMessage != "" {
			return fmt.Errorf("request failed with status %d: %s", statusCode, errMessage)
		}
	}
	bodyText := strings.TrimSpace(string(responseBody))
	if bodyText == "" {
		return fmt.Errorf("request failed with status %d", statusCode)
	}
	return fmt.Errorf("request failed with status %d: %s", statusCode, bodyText)
}
