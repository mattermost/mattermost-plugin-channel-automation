package action

import (
	"fmt"
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
	assert.Equal(t, "Summarize: Hello world", bc.lastReq.Posts[0].Message)
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
	assert.Equal(t, "Tell me about alice", bc.lastReq.Posts[0].Message)
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
	assert.Equal(t, "Refine: previous result", bc.lastReq.Posts[0].Message)
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
					"channel_id": {"ch1", "ch2"},
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
			"channel_id": {"ch1", "ch2"},
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
