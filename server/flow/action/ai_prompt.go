package action

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-ai/public/bridgeclient"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// BridgeClient is the interface for AI bridge operations, satisfied by *bridgeclient.Client.
type BridgeClient interface {
	AgentCompletion(agent string, req bridgeclient.CompletionRequest) (string, error)
	ServiceCompletion(service string, req bridgeclient.CompletionRequest) (string, error)
}

// AIPromptAction sends a rendered prompt to an AI agent or service and stores the response.
type AIPromptAction struct {
	api          plugin.API
	bridgeClient BridgeClient
}

// NewAIPromptAction creates an AIPromptAction with the given API and bridge client.
func NewAIPromptAction(api plugin.API, bridgeClient BridgeClient) *AIPromptAction {
	return &AIPromptAction{api: api, bridgeClient: bridgeClient}
}

func (a *AIPromptAction) Type() string { return "ai_prompt" }

func (a *AIPromptAction) Execute(action *model.Action, ctx *model.FlowContext) (*model.StepOutput, error) {
	if a.bridgeClient == nil {
		return nil, fmt.Errorf("agents plugin is not installed or active")
	}

	cfg := action.AIPrompt
	if cfg == nil {
		return nil, fmt.Errorf("ai_prompt action has no ai_prompt config")
	}

	if cfg.Prompt == "" {
		return nil, fmt.Errorf("missing required config key %q", "prompt")
	}
	if cfg.ProviderType == "" {
		return nil, fmt.Errorf("missing required config key %q", "provider_type")
	}
	if cfg.ProviderID == "" {
		return nil, fmt.Errorf("missing required config key %q", "provider_id")
	}

	rendered, err := renderTemplate(cfg.Prompt, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	a.api.LogDebug("AI prompt action: rendered prompt",
		"action_id", action.ID,
		"provider_type", cfg.ProviderType,
		"provider_id", cfg.ProviderID,
		"rendered_prompt_length", fmt.Sprintf("%d", len(rendered)),
	)

	var channelID string
	if ctx.Trigger.Channel != nil {
		channelID = ctx.Trigger.Channel.Id
	}

	req := bridgeclient.CompletionRequest{
		Posts: []bridgeclient.Post{
			{Role: "user", Message: rendered},
		},
		UserID:    ctx.CreatedBy,
		ChannelID: channelID,
	}

	var response string
	switch cfg.ProviderType {
	case "agent":
		response, err = a.bridgeClient.AgentCompletion(cfg.ProviderID, req)
	case "service":
		response, err = a.bridgeClient.ServiceCompletion(cfg.ProviderID, req)
	default:
		return nil, fmt.Errorf("unsupported provider_type %q, must be \"agent\" or \"service\"", cfg.ProviderType)
	}
	if err != nil {
		a.api.LogDebug("AI prompt action: completion failed",
			"action_id", action.ID,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("AI completion failed: %w", err)
	}

	a.api.LogDebug("AI prompt action: completion succeeded",
		"action_id", action.ID,
		"response_length", fmt.Sprintf("%d", len(response)),
	)

	return &model.StepOutput{
		Message: response,
	}, nil
}
