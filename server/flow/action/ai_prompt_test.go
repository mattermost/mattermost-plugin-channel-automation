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
	// Posts: [trigger metadata (system), user-generated post content (user), user prompt (user)]
	require.Len(t, bc.lastReq.Posts, 3)
	assert.Equal(t, "system", bc.lastReq.Posts[0].Role)
	assert.Contains(t, bc.lastReq.Posts[0].Message, "<trigger_context>")
	assert.NotContains(t, bc.lastReq.Posts[0].Message, "Hello world") // post message must NOT be in system prompt
	assert.Equal(t, "user", bc.lastReq.Posts[1].Role)
	assert.Contains(t, bc.lastReq.Posts[1].Message, "Hello world")
	assert.Contains(t, bc.lastReq.Posts[1].Message, "<user_data>")
	assert.Equal(t, "user", bc.lastReq.Posts[2].Role)
	assert.Equal(t, "Summarize: Hello world", bc.lastReq.Posts[2].Message)
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
			User: &model.SafeUser{Id: "user1", Username: "alice"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "Service response", output.Message)
	assert.Equal(t, "openai", bc.lastService)
	// Posts: [trigger metadata (system), user-generated content (user), user prompt (user)]
	require.Len(t, bc.lastReq.Posts, 3)
	assert.Equal(t, "system", bc.lastReq.Posts[0].Role)
	assert.Contains(t, bc.lastReq.Posts[0].Message, "Triggering User ID: user1")
	assert.NotContains(t, bc.lastReq.Posts[0].Message, "alice")
	assert.Equal(t, "user", bc.lastReq.Posts[1].Role)
	assert.Contains(t, bc.lastReq.Posts[1].Message, "Triggering Username: alice")
	assert.Equal(t, "user", bc.lastReq.Posts[2].Role)
	assert.Equal(t, "Tell me about alice", bc.lastReq.Posts[2].Message)
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

func TestAIPromptAction_Execute_AllowedTools(t *testing.T) {
	api := newTestAPI()
	bc := &mockBridgeClient{agentResponse: "tool result"}
	a := NewAIPromptAction(api, bc)

	act := &model.Action{
		ID: "ai1",
		AIPrompt: &model.AIPromptActionConfig{
			Prompt:       "Do something",
			ProviderType: "agent",
			ProviderID:   "ai-bot",
			AllowedTools: bridgeclient.AllowedToolsList{
				{ServerOrigin: "https://mcp.example", Name: "search"},
				{Name: "create_post"},
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
	assert.Equal(t, []bridgeclient.AllowedToolRef{
		{ServerOrigin: "https://mcp.example", Name: "search"},
		{Name: "create_post"},
	}, bc.lastReq.AllowedTools)
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
			User: &model.SafeUser{Id: "user1", Username: "alice"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "response", output.Message)
	// Posts: [custom system prompt, trigger metadata (system), user-generated content (user), user prompt (user)]
	require.Len(t, bc.lastReq.Posts, 4)
	assert.Equal(t, "system", bc.lastReq.Posts[0].Role)
	assert.Equal(t, "You are a helpful assistant for alice.", bc.lastReq.Posts[0].Message)
	assert.Equal(t, "system", bc.lastReq.Posts[1].Role)
	assert.Contains(t, bc.lastReq.Posts[1].Message, "<trigger_context>")
	assert.Contains(t, bc.lastReq.Posts[1].Message, "Triggering User ID: user1")
	assert.NotContains(t, bc.lastReq.Posts[1].Message, "alice")
	assert.Equal(t, "user", bc.lastReq.Posts[2].Role)
	assert.Contains(t, bc.lastReq.Posts[2].Message, "Triggering Username: alice")
	assert.Equal(t, "user", bc.lastReq.Posts[3].Role)
	assert.Equal(t, "Summarize this.", bc.lastReq.Posts[3].Message)
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
	assert.NotContains(t, bc.lastReq.Posts[0].Message, "<trigger_context>")
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
		meta, userContent := buildTriggerContext(model.TriggerData{})
		assert.Empty(t, meta)
		assert.Empty(t, userContent)
	})

	t.Run("post trigger separates metadata from user content", func(t *testing.T) {
		meta, userContent := buildTriggerContext(model.TriggerData{
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
		// Metadata (system-safe) should contain only IDs
		assert.Contains(t, meta, "<trigger_context>")
		assert.Contains(t, meta, "Post ID: post123")
		assert.Contains(t, meta, "Thread ID: thread456")
		assert.Contains(t, meta, "Channel ID: chan789")
		assert.Contains(t, meta, "Triggering User ID: user1")
		assert.Equal(t, 1, strings.Count(meta, "Channel ID:"))
		assert.Contains(t, meta, "Channel Name: incidents")
		// User-generated fields must NOT be in metadata
		assert.NotContains(t, meta, "Alert: server is down")
		assert.NotContains(t, meta, "Incidents")
		assert.NotContains(t, meta, "sysadmin")

		// User content should contain user-generated fields with untrusted warning
		assert.Contains(t, userContent, "Alert: server is down")
		assert.Contains(t, userContent, "Channel Display Name: Incidents")
		assert.NotContains(t, userContent, "Channel Name: incidents")
		assert.Contains(t, userContent, "Triggering Username: sysadmin")
		assert.Contains(t, userContent, "<user_data>")
	})

	t.Run("schedule trigger", func(t *testing.T) {
		meta, userContent := buildTriggerContext(model.TriggerData{
			Schedule: &model.ScheduleInfo{
				Interval: "daily",
				FiredAt:  1700000000000,
			},
		})
		assert.Contains(t, meta, "<trigger_context>")
		assert.Contains(t, meta, "Schedule Interval: daily")
		assert.Contains(t, meta, "Fired At: 1700000000000")
		assert.NotContains(t, meta, "Post ID")
		assert.NotContains(t, meta, "Triggering User")
		assert.Empty(t, userContent)
	})

	t.Run("channel only trigger", func(t *testing.T) {
		meta, userContent := buildTriggerContext(model.TriggerData{
			Channel: &model.SafeChannel{
				Id:   "chan1",
				Name: "general",
			},
		})
		assert.Contains(t, meta, "Channel ID: chan1")
		assert.Contains(t, meta, "Channel Name: general")
		assert.Empty(t, userContent)
	})

	t.Run("post with empty message produces no user content", func(t *testing.T) {
		meta, userContent := buildTriggerContext(model.TriggerData{
			Post: &model.SafePost{
				Id: "post123",
			},
		})
		assert.Contains(t, meta, "Post ID: post123")
		assert.Empty(t, userContent)
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

	// Should have: trigger metadata (system) + user-generated post (user) + user prompt (user)
	require.Len(t, bc.lastReq.Posts, 3)

	// System message: only trusted IDs
	triggerMeta := bc.lastReq.Posts[0]
	assert.Equal(t, "system", triggerMeta.Role)
	assert.Contains(t, triggerMeta.Message, "Post ID: post123")
	assert.Contains(t, triggerMeta.Message, "Thread ID: thread456")
	assert.Contains(t, triggerMeta.Message, "Channel ID: chan789")
	assert.Contains(t, triggerMeta.Message, "Triggering User ID: user1")
	assert.Contains(t, triggerMeta.Message, "Complete only the specific task")
	assert.Contains(t, triggerMeta.Message, "Channel Name: incidents")
	// User-generated fields must NOT be in system prompt
	assert.NotContains(t, triggerMeta.Message, "Postgres is down in production")
	assert.NotContains(t, triggerMeta.Message, "Incidents")
	assert.NotContains(t, triggerMeta.Message, "sysadmin")

	// User message: all user-generated content with untrusted warning
	userGenerated := bc.lastReq.Posts[1]
	assert.Equal(t, "user", userGenerated.Role)
	assert.Contains(t, userGenerated.Message, "Postgres is down in production")
	assert.Contains(t, userGenerated.Message, "Channel Display Name: Incidents")
	assert.Contains(t, userGenerated.Message, "Triggering Username: sysadmin")
	assert.Contains(t, userGenerated.Message, "<user_data>")

	assert.Equal(t, "user", bc.lastReq.Posts[2].Role)
	assert.Equal(t, "Handle this incident", bc.lastReq.Posts[2].Message)
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
