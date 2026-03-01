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
			User: &model.SafeUser{Username: "alice"},
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
	api.On("CreatePost", mock.Anything).Return(&mmmodel.Post{
		Id:        "new-post-id",
		ChannelId: "ch2",
		Message:   "Hello alice",
	}, nil)

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID:        "act1",
		Type:      "send_message",
		ChannelID: "ch2",
		Body:      "Hello {{.Trigger.User.Username}}",
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
	assert.Equal(t, "new-post-id", output.PostID)
	assert.Equal(t, "ch2", output.ChannelID)
	assert.Equal(t, "Hello alice", output.Message)

	api.AssertCalled(t, "CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.UserId == "bot-id" && p.ChannelId == "ch2" && p.Message == "Hello alice"
	}))
}

func TestSendMessageAction_Execute_CreatePostFailure(t *testing.T) {
	api := &plugintest.API{}
	api.On("CreatePost", mock.Anything).Return(nil, mmmodel.NewAppError("CreatePost", "error", nil, "", 500))

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID:        "act1",
		Type:      "send_message",
		ChannelID: "ch2",
		Body:      "Hello",
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

func TestSendMessageAction_Execute_BadTemplate(t *testing.T) {
	api := &plugintest.API{}

	a := NewSendMessageAction(api, "bot-id")
	act := &model.Action{
		ID:        "act1",
		Type:      "send_message",
		ChannelID: "ch2",
		Body:      "{{.Invalid",
	}
	ctx := &model.FlowContext{
		Trigger: model.TriggerData{},
		Steps:   make(map[string]model.StepOutput),
	}

	output, err := a.Execute(act, ctx)
	require.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to render template")
}
