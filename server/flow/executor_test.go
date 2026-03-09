package flow

import (
	"testing"

	"github.com/mattermost/mattermost-plugin-ai/public/bridgeclient"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/action"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// mockBridgeClient implements action.BridgeClient for executor tests.
type mockBridgeClient struct {
	agentResponse string
	lastReq       bridgeclient.CompletionRequest
}

func (m *mockBridgeClient) AgentCompletion(agent string, req bridgeclient.CompletionRequest) (string, error) {
	m.lastReq = req
	return m.agentResponse, nil
}

func (m *mockBridgeClient) ServiceCompletion(_ string, req bridgeclient.CompletionRequest) (string, error) {
	m.lastReq = req
	return m.agentResponse, nil
}

func TestFlowExecutor_SingleAction(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator1", "ch2", mmmodel.PermissionCreatePost).Return(true)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "p1",
		ChannelId: "ch2",
		Message:   "Hello alice",
	}, nil)

	registry := NewRegistry()
	registry.RegisterAction(action.NewSendMessageAction(api, "bot"))

	executor := NewFlowExecutor(registry)

	f := &model.Flow{
		ID:        "flow1",
		Name:      "Test",
		CreatedBy: "creator1",
		Actions: []model.Action{
			{ID: "act1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch2", Body: "Hello {{.Trigger.User.Username}}"}},
		},
	}
	triggerData := model.TriggerData{
		Post:    &model.SafePost{Id: "post1", ChannelId: "ch1"},
		Channel: &model.SafeChannel{Id: "ch1"},
		User:    &model.SafeUser{Id: "user1", Username: "alice"},
	}

	_, err := executor.Execute(f, triggerData)
	require.NoError(t, err)
	api.AssertCalled(t, "CreatePost", mock.Anything)
}

func TestFlowExecutor_MultiAction_CumulativeContext(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator1", mock.Anything, mmmodel.PermissionCreatePost).Return(true)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "p1",
		ChannelId: "ch2",
		Message:   "msg1",
	}, nil).Once()
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "p2",
		ChannelId: "ch3",
		Message:   "msg2",
	}, nil).Once()

	registry := NewRegistry()
	registry.RegisterAction(action.NewSendMessageAction(api, "bot"))

	executor := NewFlowExecutor(registry)

	f := &model.Flow{
		ID:        "flow1",
		Name:      "Test",
		CreatedBy: "creator1",
		Actions: []model.Action{
			{ID: "act1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch2", Body: "msg1"}},
			{ID: "act2", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch3", Body: "msg2"}},
		},
	}
	triggerData := model.TriggerData{
		Post: &model.SafePost{Id: "post1", ChannelId: "ch1"},
		User: &model.SafeUser{Id: "user1", Username: "alice"},
	}

	_, err := executor.Execute(f, triggerData)
	require.NoError(t, err)
	api.AssertNumberOfCalls(t, "CreatePost", 2)
}

func TestFlowExecutor_FirstFailureStops(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator1", mock.Anything, mmmodel.PermissionCreatePost).Return(true)
	api.On("CreatePost", mock.Anything).Return(nil, mmmodel.NewAppError("CreatePost", "error", nil, "", 500)).Once()

	registry := NewRegistry()
	registry.RegisterAction(action.NewSendMessageAction(api, "bot"))

	executor := NewFlowExecutor(registry)

	f := &model.Flow{
		ID:        "flow1",
		Name:      "Test",
		CreatedBy: "creator1",
		Actions: []model.Action{
			{ID: "act1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch2", Body: "msg1"}},
			{ID: "act2", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch3", Body: "msg2"}},
		},
	}
	triggerData := model.TriggerData{
		Post: &model.SafePost{Id: "post1", ChannelId: "ch1"},
		User: &model.SafeUser{Id: "user1"},
	}

	_, err := executor.Execute(f, triggerData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `action "act1" failed`)
	// Second action should never be called.
	api.AssertNumberOfCalls(t, "CreatePost", 1)
}

func TestFlowExecutor_ChainedAIPromptThenSendMessage(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator1", "ch2", mmmodel.PermissionCreatePost).Return(true)
	api.On("CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.Message == "AI said: hello from AI"
	})).Return(&mmmodel.Post{
		Id:        "p1",
		ChannelId: "ch2",
		Message:   "AI said: hello from AI",
	}, nil)
	api.On("LogDebug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("LogDebug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	// Mock bridge client for AI action
	bc := &mockBridgeClient{agentResponse: "hello from AI"}

	registry := NewRegistry()
	registry.RegisterAction(action.NewSendMessageAction(api, "bot"))
	registry.RegisterAction(action.NewAIPromptAction(api, bc))

	executor := NewFlowExecutor(registry)

	f := &model.Flow{
		ID:        "flow1",
		Name:      "Chained AI Test",
		CreatedBy: "creator1",
		Actions: []model.Action{
			{
				ID: "ai_step",
				AIPrompt: &model.AIPromptActionConfig{
					Prompt:       "Summarize: {{.Trigger.Post.Message}}",
					ProviderType: "agent",
					ProviderID:   "ai-bot",
				},
			},
			{
				ID: "send_step",
				SendMessage: &model.SendMessageActionConfig{
					ChannelID: "ch2",
					Body:      `AI said: {{(index .Steps "ai_step").Message}}`,
				},
			},
		},
	}
	triggerData := model.TriggerData{
		Post:    &model.SafePost{Id: "post1", ChannelId: "ch1", Message: "some text"},
		Channel: &model.SafeChannel{Id: "ch1"},
		User:    &model.SafeUser{Id: "user1", Username: "alice"},
	}

	_, err := executor.Execute(f, triggerData)
	require.NoError(t, err)
	api.AssertCalled(t, "CreatePost", mock.Anything)
	assert.Equal(t, "Summarize: some text", bc.lastReq.Posts[0].Message)
}

func TestFlowExecutor_UnknownActionType(t *testing.T) {
	registry := NewRegistry()
	executor := NewFlowExecutor(registry)

	f := &model.Flow{
		ID:   "flow1",
		Name: "Test",
		Actions: []model.Action{
			{ID: "act1"},
		},
	}
	triggerData := model.TriggerData{
		Post: &model.SafePost{Id: "post1", ChannelId: "ch1"},
	}

	_, err := executor.Execute(f, triggerData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown action type")
}

func TestFlowExecutor_PermissionDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator1", "ch2", mmmodel.PermissionCreatePost).Return(false)

	registry := NewRegistry()
	registry.RegisterAction(action.NewSendMessageAction(api, "bot"))

	executor := NewFlowExecutor(registry)

	f := &model.Flow{
		ID:        "flow1",
		Name:      "Test",
		CreatedBy: "creator1",
		Actions: []model.Action{
			{ID: "act1", SendMessage: &model.SendMessageActionConfig{ChannelID: "ch2", Body: "msg"}},
		},
	}
	triggerData := model.TriggerData{
		Post: &model.SafePost{Id: "post1", ChannelId: "ch1"},
		User: &model.SafeUser{Id: "user1"},
	}

	_, err := executor.Execute(f, triggerData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have permission to post in channel")
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}
