package action

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-plugin-ai/public/bridgeclient"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

const completionScopeInstruction = "Complete only the specific task described in the user prompt below, then provide your final response. " +
	"Do not take additional follow-up actions beyond what was explicitly requested."

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

	var posts []bridgeclient.Post
	if cfg.SystemPrompt != "" {
		renderedSystem, sysErr := renderTemplate(cfg.SystemPrompt, ctx)
		if sysErr != nil {
			return nil, fmt.Errorf("failed to render system prompt template: %w", sysErr)
		}
		posts = append(posts, bridgeclient.Post{Role: "system", Message: renderedSystem})
	}
	contextMsg := buildTriggerContext(ctx.Trigger)
	scopeMsg := completionScopeInstruction
	if contextMsg != "" {
		scopeMsg = contextMsg + "\n\n" + scopeMsg
	}
	posts = append(posts, bridgeclient.Post{Role: "system", Message: scopeMsg})
	posts = append(posts, bridgeclient.Post{Role: "user", Message: rendered})

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
		Posts:     posts,
		UserID:    ctx.CreatedBy,
		ChannelID: channelID,
	}

	if len(cfg.AllowedTools) > 0 {
		req.AllowedTools = cfg.AllowedTools
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

// buildTriggerContext builds a structured context string from trigger data
// so the AI agent knows what triggered the flow without requiring template variables.
func buildTriggerContext(trigger model.TriggerData) string {
	var b strings.Builder

	if trigger.Post != nil {
		p := trigger.Post
		b.WriteString("Post ID: " + p.Id + "\n")
		if p.ThreadId != "" {
			b.WriteString("Thread ID: " + p.ThreadId + "\n")
		}
		if p.ChannelId != "" {
			b.WriteString("Channel ID: " + p.ChannelId + "\n")
		}
		b.WriteString("Post Message:\n" + p.Message + "\n")
	}

	if trigger.Channel != nil {
		ch := trigger.Channel
		if trigger.Post == nil && ch.Id != "" {
			b.WriteString("Channel ID: " + ch.Id + "\n")
		}
		if ch.Name != "" {
			b.WriteString("Channel Name: " + ch.Name + "\n")
		}
		if ch.DisplayName != "" {
			b.WriteString("Channel Display Name: " + ch.DisplayName + "\n")
		}
	}

	if trigger.User != nil {
		u := trigger.User
		name := u.Username
		if u.Id != "" {
			name += " (ID: " + u.Id + ")"
		}
		b.WriteString("Triggering User: " + name + "\n")
	}

	if trigger.Schedule != nil {
		s := trigger.Schedule
		if s.Interval != "" {
			b.WriteString("Schedule Interval: " + s.Interval + "\n")
		}
		b.WriteString(fmt.Sprintf("Fired At: %d\n", s.FiredAt))
	}

	content := strings.TrimSpace(b.String())
	if content == "" {
		return ""
	}

	return "[Trigger Context]\nThis automation was triggered by the following event:\n\n" + content
}
