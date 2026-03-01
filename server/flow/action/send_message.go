package action

import (
	"fmt"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// SendMessageAction posts a message to a channel using a Go text/template body.
type SendMessageAction struct {
	api       plugin.API
	botUserID string
}

// NewSendMessageAction creates a SendMessageAction with the given API and bot user ID.
func NewSendMessageAction(api plugin.API, botUserID string) *SendMessageAction {
	return &SendMessageAction{api: api, botUserID: botUserID}
}

func (a *SendMessageAction) Type() string { return "send_message" }

func (a *SendMessageAction) Execute(action *model.Action, ctx *model.FlowContext) (*model.StepOutput, error) {
	msg, err := renderTemplate(action.Body, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	post := &mmmodel.Post{
		UserId:    a.botUserID,
		ChannelId: action.ChannelID,
		Message:   msg,
	}

	created, appErr := a.api.CreatePost(post)
	if appErr != nil {
		return nil, fmt.Errorf("failed to create post: %s", appErr.Error())
	}
	if created == nil {
		return nil, fmt.Errorf("CreatePost returned nil post without error")
	}

	return &model.StepOutput{
		PostID:    created.Id,
		ChannelID: created.ChannelId,
		Message:   msg,
	}, nil
}
