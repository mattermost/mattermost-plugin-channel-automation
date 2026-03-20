package action

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestSendDMAction_Type(t *testing.T) {
	a := NewSendDMAction(nil, "bot")
	assert.Equal(t, "send_dm", a.Type())
}

func TestSendDMAction_Execute_Success(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetUser", "custom-bot-id").Return(&mmmodel.User{Id: "custom-bot-id", IsBot: true}, nil)
	api.On("GetDirectChannel", "custom-bot-id", "target-user-id").Return(&mmmodel.Channel{Id: "dm-channel-id"}, nil)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "new-post-id",
		ChannelId: "dm-channel-id",
		Message:   "Hello alice",
	}, nil)

	a := NewSendDMAction(api, "bot-id")
	act := &model.Action{
		ID: "dm1",
		SendDM: &model.SendDMActionConfig{
			UserID:  "target-user-id",
			Body:    "Hello {{.Trigger.User.Username}}",
			AsBotID: "custom-bot-id",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger: model.TriggerData{
			User: &model.SafeUser{Username: "alice"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "new-post-id", output.PostID)
	assert.Equal(t, "dm-channel-id", output.ChannelID)
	assert.Equal(t, "Hello alice", output.Message)

	api.AssertCalled(t, "GetDirectChannel", "custom-bot-id", "target-user-id")
	api.AssertCalled(t, "CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.UserId == "custom-bot-id" && p.ChannelId == "dm-channel-id" && p.Message == "Hello alice"
	}))
}

func TestSendDMAction_Execute_NilConfig(t *testing.T) {
	a := NewSendDMAction(nil, "bot-id")
	act := &model.Action{ID: "dm1"}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "send_dm action has no send_dm config")
}

func TestSendDMAction_Execute_TemplatedFields(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetUser", "bot-from-template").Return(&mmmodel.User{Id: "bot-from-template", IsBot: true}, nil)
	api.On("GetDirectChannel", "bot-from-template", "user-from-template").Return(&mmmodel.Channel{Id: "dm-ch"}, nil)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "post-id",
		ChannelId: "dm-ch",
		Message:   "Hi alice",
	}, nil)

	a := NewSendDMAction(api, "bot-id")
	act := &model.Action{
		ID: "dm1",
		SendDM: &model.SendDMActionConfig{
			UserID:  "{{.Trigger.User.Id}}",
			Body:    "Hi {{.Trigger.User.Username}}",
			AsBotID: "{{.Trigger.Channel.Id}}",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{
			User:    &model.SafeUser{Id: "user-from-template", Username: "alice"},
			Channel: &model.SafeChannel{Id: "bot-from-template"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "dm-ch", output.ChannelID)

	api.AssertCalled(t, "GetUser", "bot-from-template")
	api.AssertCalled(t, "GetDirectChannel", "bot-from-template", "user-from-template")
}

func TestSendDMAction_Execute_BadUserIDTemplate(t *testing.T) {
	a := NewSendDMAction(nil, "bot-id")
	act := &model.Action{
		ID: "dm1",
		SendDM: &model.SendDMActionConfig{
			UserID:  "{{.Invalid",
			Body:    "hello",
			AsBotID: "bot-id",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to render user_id template")
}

func TestSendDMAction_Execute_BadBodyTemplate(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetUser", "bot-id").Return(&mmmodel.User{Id: "bot-id", IsBot: true}, nil)

	a := NewSendDMAction(api, "bot-id")
	act := &model.Action{
		ID: "dm1",
		SendDM: &model.SendDMActionConfig{
			UserID:  "target-user",
			Body:    "{{.Invalid",
			AsBotID: "bot-id",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to render body template")
}

func TestSendDMAction_Execute_BadAsBotIDTemplate(t *testing.T) {
	a := NewSendDMAction(nil, "bot-id")
	act := &model.Action{
		ID: "dm1",
		SendDM: &model.SendDMActionConfig{
			UserID:  "target-user",
			Body:    "hello",
			AsBotID: "{{.Invalid",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to render as_bot_id template")
}

func TestSendDMAction_Execute_BotNotFound(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetUser", "nonexistent-bot").Return(nil, mmmodel.NewAppError("GetUser", "not_found", nil, "", 404))

	a := NewSendDMAction(api, "bot-id")
	act := &model.Action{
		ID: "dm1",
		SendDM: &model.SendDMActionConfig{
			UserID:  "target-user",
			Body:    "hello",
			AsBotID: "nonexistent-bot",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to get bot user")
}

func TestSendDMAction_Execute_NotABot(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetUser", "human-user").Return(&mmmodel.User{Id: "human-user", IsBot: false}, nil)

	a := NewSendDMAction(api, "bot-id")
	act := &model.Action{
		ID: "dm1",
		SendDM: &model.SendDMActionConfig{
			UserID:  "target-user",
			Body:    "hello",
			AsBotID: "human-user",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "is not a bot")
	api.AssertNotCalled(t, "GetDirectChannel", mock.Anything, mock.Anything)
}

func TestSendDMAction_Execute_GetDirectChannelFails(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetUser", "bot-id").Return(&mmmodel.User{Id: "bot-id", IsBot: true}, nil)
	api.On("GetDirectChannel", "bot-id", "target-user").Return(nil, mmmodel.NewAppError("GetDirectChannel", "error", nil, "", 500))

	a := NewSendDMAction(api, "bot-id")
	act := &model.Action{
		ID: "dm1",
		SendDM: &model.SendDMActionConfig{
			UserID:  "target-user",
			Body:    "hello",
			AsBotID: "bot-id",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to get DM channel")
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestSendDMAction_Execute_CreatePostFails(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetUser", "bot-id").Return(&mmmodel.User{Id: "bot-id", IsBot: true}, nil)
	api.On("GetDirectChannel", "bot-id", "target-user").Return(&mmmodel.Channel{Id: "dm-ch"}, nil)
	api.On("CreatePost", mock.Anything).Return(nil, mmmodel.NewAppError("CreatePost", "error", nil, "", 500))

	a := NewSendDMAction(api, "bot-id")
	act := &model.Action{
		ID: "dm1",
		SendDM: &model.SendDMActionConfig{
			UserID:  "target-user",
			Body:    "hello",
			AsBotID: "bot-id",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to create post")
}
