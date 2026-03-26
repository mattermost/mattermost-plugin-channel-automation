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

func TestRenderTemplate_SimpleVariable(t *testing.T) {
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{
			User: &model.SafeUser{Id: "user1", Username: "alice"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	result, err := renderTemplate("Hello {{.Trigger.User.Username}}", ctx)
	require.NoError(t, err)
	assert.Equal(t, "Hello alice", result)
}

func TestRenderTemplate_Conditional(t *testing.T) {
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{
			Channel: &model.SafeChannel{DisplayName: "General"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	tmpl := `{{if .Trigger.Channel}}Channel: {{.Trigger.Channel.DisplayName}}{{else}}No channel{{end}}`
	result, err := renderTemplate(tmpl, ctx)
	require.NoError(t, err)
	assert.Equal(t, "Channel: General", result)
}

func TestRenderTemplate_StepReference(t *testing.T) {
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps: map[string]model.StepOutput{
			"step1": {PostID: "p1", Message: "prev message"},
		},
	}

	tmpl := `Previous: {{(index .Steps "step1").Message}}`
	result, err := renderTemplate(tmpl, ctx)
	require.NoError(t, err)
	assert.Equal(t, "Previous: prev message", result)
}

func TestRenderTemplate_InvalidTemplate(t *testing.T) {
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	_, err := renderTemplate("{{.Invalid", ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse template")
}

func TestRenderTemplate_ExecutionError(t *testing.T) {
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	_, err := renderTemplate("{{.Trigger.Post.Message}}", ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute template")
}

func TestSendMessageAction_Type(t *testing.T) {
	a := NewSendMessageAction(nil, "bot")
	assert.Equal(t, "send_message", a.Type())
}

func TestSendMessageAction_Execute_Success(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch2", mmmodel.PermissionCreatePost).Return(true)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "new-post-id",
		ChannelId: "ch2",
		Message:   "Hello alice",
	}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch2",
			Body:      "Hello {{.Trigger.User.Username}}",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger: model.TriggerData{
			User: &model.SafeUser{Id: "user1", Username: "alice"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "new-post-id", output.PostID)
	assert.Equal(t, "ch2", output.ChannelID)
	assert.Equal(t, "Hello alice", output.Message)

	api.AssertCalled(t, "CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.UserId == "bot-id" && p.ChannelId == "ch2" && p.Message == "Hello alice"
	}))
}

func TestSendMessageAction_Execute_TemplatedChannelID(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "trigger-ch", mmmodel.PermissionCreatePost).Return(true)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "new-post-id",
		ChannelId: "trigger-ch",
		Message:   "hello",
	}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "{{.Trigger.Channel.Id}}",
			Body:      "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger: model.TriggerData{
			Channel: &model.SafeChannel{Id: "trigger-ch"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "trigger-ch", output.ChannelID)

	api.AssertCalled(t, "CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.ChannelId == "trigger-ch"
	}))
}

func TestSendMessageAction_Execute_ReplyToPostID(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch1", mmmodel.PermissionCreatePost).Return(true)
	api.On("GetPost", "parent-post-id").Return(&mmmodel.Post{Id: "parent-post-id"}, nil)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "reply-post-id",
		ChannelId: "ch1",
		RootId:    "parent-post-id",
		Message:   "threaded reply",
	}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID:     "ch1",
			ReplyToPostID: "parent-post-id",
			Body:          "threaded reply",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "reply-post-id", output.PostID)

	api.AssertCalled(t, "CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.RootId == "parent-post-id" && p.ChannelId == "ch1"
	}))
}

func TestSendMessageAction_Execute_ReplyToPostID_Templated(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch1", mmmodel.PermissionCreatePost).Return(true)
	api.On("GetPost", "trigger-post-id").Return(&mmmodel.Post{Id: "trigger-post-id"}, nil)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "reply-post-id",
		ChannelId: "ch1",
		RootId:    "trigger-post-id",
		Message:   "reply",
	}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID:     "ch1",
			ReplyToPostID: "{{.Trigger.Post.Id}}",
			Body:          "reply",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger: model.TriggerData{
			Post: &model.SafePost{Id: "trigger-post-id", ChannelId: "ch1"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)

	api.AssertCalled(t, "CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.RootId == "trigger-post-id"
	}))
}

func TestSendMessageAction_Execute_BadReplyToPostIDTemplate(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch1", mmmodel.PermissionCreatePost).Return(true)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID:     "ch1",
			ReplyToPostID: "{{.Invalid",
			Body:          "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to render reply_to_post_id template")
}

func TestSendMessageAction_Execute_EmptyReplyToPostID(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch1", mmmodel.PermissionCreatePost).Return(true)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "new-post-id",
		ChannelId: "ch1",
		Message:   "hello",
	}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch1",
			Body:      "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)

	api.AssertCalled(t, "CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.RootId == ""
	}))
}

func TestSendMessageAction_Execute_BadChannelIDTemplate(t *testing.T) {
	api := &plugintest.API{}

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "{{.Invalid",
			Body:      "hello",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to render channel_id template")
}

func TestSendMessageAction_Execute_CreatePostFailure(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch2", mmmodel.PermissionCreatePost).Return(true)
	api.On("CreatePost", mock.Anything).Return(nil, mmmodel.NewAppError("CreatePost", "error", nil, "", 500))

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch2",
			Body:      "Hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to create post")
}

func TestSendMessageAction_Execute_BadTemplate(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch2", mmmodel.PermissionCreatePost).Return(true)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch2",
			Body:      "{{.Invalid",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to render template")
}

func TestSendMessageAction_Execute_PermissionDenied(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch2", mmmodel.PermissionCreatePost).Return(false)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch2",
			Body:      "Hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "does not have permission to post in channel")
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestSendMessageAction_Execute_EmptyCreatedBy(t *testing.T) {
	api := &plugintest.API{}

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch2",
			Body:      "Hello",
		},
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "flow has no creator")
	api.AssertNotCalled(t, "HasPermissionToChannel", mock.Anything, mock.Anything, mock.Anything)
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestSendMessageAction_Execute_ReplyToPostID_ResolvesRootFromChild(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch1", mmmodel.PermissionCreatePost).Return(true)
	api.On("GetPost", "child-post-id").Return(&mmmodel.Post{Id: "child-post-id", RootId: "root-post-id"}, nil)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "reply-post-id",
		ChannelId: "ch1",
		RootId:    "root-post-id",
		Message:   "threaded reply",
	}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID:     "ch1",
			ReplyToPostID: "child-post-id",
			Body:          "threaded reply",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)

	api.AssertCalled(t, "CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.RootId == "root-post-id"
	}))
}

func TestSendMessageAction_Execute_ReplyToPostID_GetPostFails(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch1", mmmodel.PermissionCreatePost).Return(true)
	api.On("GetPost", "missing-post-id").Return(nil, mmmodel.NewAppError("GetPost", "not_found", nil, "", 404))

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID:     "ch1",
			ReplyToPostID: "missing-post-id",
			Body:          "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to get post")
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestSendMessageAction_Execute_ReplyToPostID_Templated_ResolvesRoot(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch1", mmmodel.PermissionCreatePost).Return(true)
	api.On("GetPost", "child-post-id").Return(&mmmodel.Post{Id: "child-post-id", RootId: "root-post-id"}, nil)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "reply-post-id",
		ChannelId: "ch1",
		RootId:    "root-post-id",
		Message:   "reply",
	}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID:     "ch1",
			ReplyToPostID: "{{.Trigger.Post.Id}}",
			Body:          "reply",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger: model.TriggerData{
			Post: &model.SafePost{Id: "child-post-id", ChannelId: "ch1"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)

	api.AssertCalled(t, "CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.RootId == "root-post-id"
	}))
}

func TestSendMessageAction_Execute_AsBotID_ValidBot(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch1", mmmodel.PermissionCreatePost).Return(true)
	api.On("GetUser", "custom-bot-id").Return(&mmmodel.User{Id: "custom-bot-id", IsBot: true}, nil)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "new-post-id",
		ChannelId: "ch1",
		Message:   "hello",
	}, nil)

	a := NewSendMessageAction(api, "default-bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch1",
			AsBotID:   "custom-bot-id",
			Body:      "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)

	api.AssertCalled(t, "CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.UserId == "custom-bot-id"
	}))
}

func TestSendMessageAction_Execute_AsBotID_NotABot(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch1", mmmodel.PermissionCreatePost).Return(true)
	api.On("GetUser", "human-user-id").Return(&mmmodel.User{Id: "human-user-id", IsBot: false}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch1",
			AsBotID:   "human-user-id",
			Body:      "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "is not a bot")
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestSendMessageAction_Execute_AsBotID_UserNotFound(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch1", mmmodel.PermissionCreatePost).Return(true)
	api.On("GetUser", "nonexistent-id").Return(nil, mmmodel.NewAppError("GetUser", "not_found", nil, "", 404))

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch1",
			AsBotID:   "nonexistent-id",
			Body:      "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to get bot user")
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestSendMessageAction_Execute_ChannelGuardrail_BlocksDifferentChannel(t *testing.T) {
	api := &plugintest.API{}

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch-other",
			Body:      "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger: model.TriggerData{
			Channel: &model.SafeChannel{Id: "ch-trigger"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "restricted to the triggering channel")
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestSendMessageAction_Execute_ChannelGuardrail_AllowsSameChannel(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "ch-trigger", mmmodel.PermissionCreatePost).Return(true)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "new-post-id",
		ChannelId: "ch-trigger",
		Message:   "hello",
	}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "ch-trigger",
			Body:      "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger: model.TriggerData{
			Channel: &model.SafeChannel{Id: "ch-trigger"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "ch-trigger", output.ChannelID)
}

func TestSendMessageAction_Execute_ChannelGuardrail_SkippedWhenNoTriggerChannel(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "any-ch", mmmodel.PermissionCreatePost).Return(true)
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "new-post-id",
		ChannelId: "any-ch",
		Message:   "hello",
	}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "any-ch",
			Body:      "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger:   model.TriggerData{},
		Steps:     make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "any-ch", output.ChannelID)
}

func TestSendMessageAction_Execute_PermissionDenied_TemplatedChannel(t *testing.T) {
	api := &plugintest.API{}
	api.On("HasPermissionToChannel", "creator-id", "resolved-ch", mmmodel.PermissionCreatePost).Return(false)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID: "act1",
		SendMessage: &model.SendMessageActionConfig{
			ChannelID: "{{.Trigger.Channel.Id}}",
			Body:      "hello",
		},
	}
	ctx := &model.FlowContext{
		CreatedBy: "creator-id",
		Trigger: model.TriggerData{
			Channel: &model.SafeChannel{Id: "resolved-ch"},
		},
		Steps: make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "does not have permission to post in channel")
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}
