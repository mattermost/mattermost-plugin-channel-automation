package action

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-ai/public/bridgeclient"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// mockBridgeClient implements BridgeClient for testing.
type mockBridgeClient struct {
	agentResponse   string
	agentErr        error
	serviceResponse string
	serviceErr      error

	lastAgent   string
	lastService string
	lastReq     bridgeclient.CompletionRequest
}

func (m *mockBridgeClient) AgentCompletion(agent string, req bridgeclient.CompletionRequest) (string, error) {
	m.lastAgent = agent
	m.lastReq = req
	return m.agentResponse, m.agentErr
}

func (m *mockBridgeClient) ServiceCompletion(service string, req bridgeclient.CompletionRequest) (string, error) {
	m.lastService = service
	m.lastReq = req
	return m.serviceResponse, m.serviceErr
}

func newTestAPI() *plugintest.API {
	api := &plugintest.API{}
	api.On("LogDebug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogDebug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	return api
}

func TestAIPromptAction_Type(t *testing.T) {
	a := NewAIPromptAction(nil, nil)
	assert.Equal(t, "ai_prompt", a.Type())
}

func TestAIPromptAction_Execute_AgentSuccess(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{agentResponse: "AI says hello"}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "Summarize: {{.Trigger.Post.Message}}",
			ProviderType: "agent",
			ProviderID:   "ai-bot",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{
			Post: &model.SafePost{Message: "Hello world"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "AI says hello", output.Message)
	assert.Equal(t, "ai-bot", bc.lastAgent)
	// Posts: [trigger context (system), user prompt]
	require.Len(t, bc.lastReq.Posts, 2)
	assert.Equal(t, "system", bc.lastReq.Posts[0].Role)
	assert.Contains(t, bc.lastReq.Posts[0].Message, "[Trigger Context]")
	assert.Equal(t, "user", bc.lastReq.Posts[1].Role)
	assert.Equal(t, "Summarize: Hello world", bc.lastReq.Posts[1].Message)
}

func TestAIPromptAction_Execute_ServiceSuccess(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{serviceResponse: "Service response"}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "Tell me about {{.Trigger.User.Username}}",
			ProviderType: "service",
			ProviderID:   "openai",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{
			User: &model.SafeUser{Username: "alice"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "Service response", output.Message)
	assert.Equal(t, "openai", bc.lastService)
	// Posts: [trigger context (system), user prompt]
	require.Len(t, bc.lastReq.Posts, 2)
	assert.Equal(t, "system", bc.lastReq.Posts[0].Role)
	assert.Contains(t, bc.lastReq.Posts[0].Message, "Triggering User: alice")
	assert.Equal(t, "user", bc.lastReq.Posts[1].Role)
	assert.Equal(t, "Tell me about alice", bc.lastReq.Posts[1].Message)
}

func TestAIPromptAction_Execute_TemplateWithStepOutputs(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{agentResponse: "refined output"}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai2",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       `Refine: {{(index .Steps "step1").Message}}`,
			ProviderType: "agent",
			ProviderID:   "ai-bot",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps: map[string]model.StepOutput{
			"step1": {Message: "previous result"},
		},
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "refined output", output.Message)
	// Posts: [scope instruction (system), user prompt]
	require.Len(t, bc.lastReq.Posts, 2)
	assert.Equal(t, "system", bc.lastReq.Posts[0].Role)
	assert.Contains(t, bc.lastReq.Posts[0].Message, "Complete only the specific task")
	assert.Equal(t, "Refine: previous result", bc.lastReq.Posts[1].Message)
}

func TestAIPromptAction_Execute_MissingConfig(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{}
	a := NewAIPromptAction(api, bc)

	tests := []struct {
		name     string
		aiPrompt *model.AIPromptActionConfig
		errMsg   string
	}{
		{
			name:     "missing prompt",
			aiPrompt: &model.AIPromptActionConfig{ProviderType: "agent", ProviderID: "bot"},
			errMsg:   `missing required config key "prompt"`,
		},
		{
			name:     "missing provider_type",
			aiPrompt: &model.AIPromptActionConfig{Prompt: "hello", ProviderID: "bot"},
			errMsg:   `missing required config key "provider_type"`,
		},
		{
			name:     "missing provider_id",
			aiPrompt: &model.AIPromptActionConfig{Prompt: "hello", ProviderType: "agent"},
			errMsg:   `missing required config key "provider_id"`,
		},
		{
			name:     "nil config",
			aiPrompt: nil,
			errMsg:   `ai_prompt action has no ai_prompt config`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			act := &model.Action{ID: "ai1", AIPrompt: tc.aiPrompt}
			ctx := &model.FlowContext{Trigger: model.TriggerData{}, Steps: make(map[string]model.StepOutput)}

			output, err := a.Execute(act, ctx)
			require.Error(t, err)
			assert.Nil(t, output)
			assert.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

func TestAIPromptAction_Execute_NilBridgeClient(t *testing.T) {
	api := newTestAPI()
	a := NewAIPromptAction(api, nil)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "hello",
			ProviderType: "agent",
			ProviderID:   "bot",
		},
	}
	ctx := &model.FlowContext{Trigger: model.TriggerData{}, Steps: make(map[string]model.StepOutput)}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "agents plugin is not installed or active")
}

func TestAIPromptAction_Execute_BridgeClientError(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{agentErr: fmt.Errorf("connection refused")}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "hello",
			ProviderType: "agent",
			ProviderID:   "bot",
		},
	}
	ctx := &model.FlowContext{Trigger: model.TriggerData{}, Steps: make(map[string]model.StepOutput)}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "AI completion failed")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestAIPromptAction_Execute_BadTemplate(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "{{.Invalid",
			ProviderType: "agent",
			ProviderID:   "bot",
		},
	}
	ctx := &model.FlowContext{Trigger: model.TriggerData{}, Steps: make(map[string]model.StepOutput)}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to render template")
}

func TestAIPromptAction_Execute_AllowedToolsAndConstraints(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{agentResponse: "tool result"}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "Do something",
			ProviderType: "agent",
			ProviderID:   "ai-bot",
			AllowedTools: []string{"search", "create_post"},
			ToolConstraints: model.ToolConstraints{
				"create_post": {
					"channel_id": model.ParamConstraint{AllowedValues: []string{"ch1", "ch2"}},
				},
			},
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "tool result", output.Message)
	assert.Equal(t, []string{"search", "create_post"}, bc.lastReq.AllowedTools)
	assert.Equal(t, bridgeclient.ToolConstraints{
		"create_post": {
			"channel_id": bridgeclient.ParamConstraint{AllowedValues: []string{"ch1", "ch2"}},
		},
	}, bc.lastReq.ToolConstraints)
}

func TestAIPromptAction_Execute_ToolConstraintsWithOutputBinding(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{agentResponse: "result"}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "Do something",
			ProviderType: "agent",
			ProviderID:   "ai-bot",
			AllowedTools: []string{"list_channels", "create_post"},
			ToolConstraints: model.ToolConstraints{
				"create_post": {
					"channel_id": model.ParamConstraint{
						AllowedValues: []string{"ch1"},
						FromToolOutput: []model.OutputBinding{
							{Tool: "list_channels", Field: "channel_id"},
						},
					},
				},
			},
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "result", output.Message)
	assert.Equal(t, []string{"list_channels", "create_post"}, bc.lastReq.AllowedTools)
	assert.Equal(t, bridgeclient.ToolConstraints{
		"create_post": {
			"channel_id": bridgeclient.ParamConstraint{
				AllowedValues: []string{"ch1"},
				FromToolOutput: []bridgeclient.OutputBinding{
					{Tool: "list_channels", Field: "channel_id"},
				},
			},
		},
	}, bc.lastReq.ToolConstraints)
}

func TestAIPromptAction_Execute_NoToolFields(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{agentResponse: "ok"}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "hello",
			ProviderType: "agent",
			ProviderID:   "ai-bot",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Nil(t, bc.lastReq.AllowedTools)
	assert.Nil(t, bc.lastReq.ToolConstraints)
}

func TestAIPromptAction_Execute_SystemPromptRendered(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{agentResponse: "response"}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			SystemPrompt: "You are a helpful assistant for {{.Trigger.User.Username}}.",
			Prompt:       "Summarize this.",
			ProviderType: "agent",
			ProviderID:   "ai-bot",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{
			User: &model.SafeUser{Username: "alice"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "response", output.Message)
	// Posts: [user system prompt, trigger context (system), user prompt]
	require.Len(t, bc.lastReq.Posts, 3)
	assert.Equal(t, "system", bc.lastReq.Posts[0].Role)
	assert.Equal(t, "You are a helpful assistant for alice.", bc.lastReq.Posts[0].Message)
	assert.Equal(t, "system", bc.lastReq.Posts[1].Role)
	assert.Contains(t, bc.lastReq.Posts[1].Message, "[Trigger Context]")
	assert.Equal(t, "user", bc.lastReq.Posts[2].Role)
	assert.Equal(t, "Summarize this.", bc.lastReq.Posts[2].Message)
}

func TestAIPromptAction_Execute_EmptySystemPromptNoUserSystemPost(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{agentResponse: "response"}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "Hello",
			ProviderType: "agent",
			ProviderID:   "ai-bot",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	// Even with no user system prompt and empty trigger, scope instruction is always present
	require.Len(t, bc.lastReq.Posts, 2)
	assert.Equal(t, "system", bc.lastReq.Posts[0].Role)
	assert.Contains(t, bc.lastReq.Posts[0].Message, "Complete only the specific task")
	assert.NotContains(t, bc.lastReq.Posts[0].Message, "[Trigger Context]")
	assert.Equal(t, "user", bc.lastReq.Posts[1].Role)
	assert.Equal(t, "Hello", bc.lastReq.Posts[1].Message)
}

func TestAIPromptAction_Execute_BadSystemPromptTemplate(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			SystemPrompt: "{{.Invalid",
			Prompt:       "hello",
			ProviderType: "agent",
			ProviderID:   "bot",
		},
	}
	ctx := &model.FlowContext{Trigger: model.TriggerData{}, Steps: make(map[string]model.StepOutput)}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to render system prompt template")
}

func TestBuildTriggerContext(t *testing.T) {
	t.Run("empty trigger", func(t *testing.T) {
		got := buildTriggerContext(model.TriggerData{})
		assert.Empty(t, got)
	})

	t.Run("post trigger", func(t *testing.T) {
		got := buildTriggerContext(model.TriggerData{
			Post: &model.SafePost{
				Id:        "post123",
				ThreadId:  "thread456",
				ChannelId: "chan789",
				Message:   "Alert: server is down",
			},
			Channel: &model.SafeChannel{
				Id:          "chan789",
				Name:        "incidents",
				DisplayName: "Incidents",
			},
			User: &model.SafeUser{
				Id:       "user1",
				Username: "sysadmin",
			},
		})
		assert.Contains(t, got, "[Trigger Context]")
		assert.Contains(t, got, "Post ID: post123")
		assert.Contains(t, got, "Thread ID: thread456")
		assert.Contains(t, got, "Channel ID: chan789")
		assert.Contains(t, got, "Post Message:\nAlert: server is down")
		assert.Contains(t, got, "Channel Name: incidents")
		assert.Contains(t, got, "Channel Display Name: Incidents")
		assert.Contains(t, got, "Triggering User: sysadmin (ID: user1)")
		// Channel ID should not be duplicated (already in post section)
		assert.Equal(t, 1, strings.Count(got, "Channel ID:"))
	})

	t.Run("schedule trigger", func(t *testing.T) {
		got := buildTriggerContext(model.TriggerData{
			Schedule: &model.ScheduleInfo{
				Interval: "daily",
				FiredAt:  1700000000000,
			},
		})
		assert.Contains(t, got, "[Trigger Context]")
		assert.Contains(t, got, "Schedule Interval: daily")
		assert.Contains(t, got, "Fired At: 1700000000000")
		assert.NotContains(t, got, "Post ID")
		assert.NotContains(t, got, "Triggering User")
	})

	t.Run("channel only trigger", func(t *testing.T) {
		got := buildTriggerContext(model.TriggerData{
			Channel: &model.SafeChannel{
				Id:   "chan1",
				Name: "general",
			},
		})
		assert.Contains(t, got, "Channel ID: chan1")
		assert.Contains(t, got, "Channel Name: general")
	})
}

func TestAIPromptAction_Execute_TriggerContextInjected(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{agentResponse: "done"}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "Handle this incident",
			ProviderType: "agent",
			ProviderID:   "ai-bot",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{
			Post: &model.SafePost{
				Id:        "post123",
				ThreadId:  "thread456",
				ChannelId: "chan789",
				Message:   "Postgres is down in production",
			},
			Channel: &model.SafeChannel{
				Id:          "chan789",
				Name:        "incidents",
				DisplayName: "Incidents",
			},
			User: &model.SafeUser{
				Id:       "user1",
				Username: "sysadmin",
			},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Should have trigger context + scope instruction system message + user prompt
	require.Len(t, bc.lastReq.Posts, 2)
	triggerCtx := bc.lastReq.Posts[0]
	assert.Equal(t, "system", triggerCtx.Role)
	assert.Contains(t, triggerCtx.Message, "Post ID: post123")
	assert.Contains(t, triggerCtx.Message, "Thread ID: thread456")
	assert.Contains(t, triggerCtx.Message, "Channel ID: chan789")
	assert.Contains(t, triggerCtx.Message, "Postgres is down in production")
	assert.Contains(t, triggerCtx.Message, "Triggering User: sysadmin (ID: user1)")
	assert.Contains(t, triggerCtx.Message, "Channel Name: incidents")
	assert.Contains(t, triggerCtx.Message, "Complete only the specific task")

	assert.Equal(t, "user", bc.lastReq.Posts[1].Role)
	assert.Equal(t, "Handle this incident", bc.lastReq.Posts[1].Message)
}

func TestAIPromptAction_Execute_UnsupportedProviderType(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "hello",
			ProviderType: "unknown",
			ProviderID:   "bot",
		},
	}
	ctx := &model.FlowContext{Trigger: model.TriggerData{}, Steps: make(map[string]model.StepOutput)}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), `unsupported provider_type "unknown"`)
}
