package action

import (
	"fmt"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// SendDMAction sends a direct message to a user from a specified bot.
type SendDMAction struct {
	api       plugin.API
	botUserID string
}

// NewSendDMAction creates a SendDMAction with the given API and bot user ID.
func NewSendDMAction(api plugin.API, botUserID string) *SendDMAction {
	return &SendDMAction{api: api, botUserID: botUserID}
}

func (a *SendDMAction) Type() string { return "send_dm" }

func (a *SendDMAction) Execute(action *model.Action, ctx *model.FlowContext) (*model.StepOutput, error) {
	cfg := action.SendDM
	if cfg == nil {
		return nil, fmt.Errorf("send_dm action has no send_dm config")
	}

	targetUserID, err := renderTemplate(cfg.UserID, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to render user_id template: %w", err)
	}

	asBotID, err := renderTemplate(cfg.AsBotID, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to render as_bot_id template: %w", err)
	}

	botUser, appErr := a.api.GetUser(asBotID)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get bot user %q: %s", asBotID, appErr.Error())
	}
	if !botUser.IsBot {
		return nil, fmt.Errorf("user %q is not a bot", asBotID)
	}

	msg, err := renderTemplate(cfg.Body, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to render body template: %w", err)
	}

	dmChannel, appErr := a.api.GetDirectChannel(asBotID, targetUserID)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get DM channel: %s", appErr.Error())
	}

	post := &mmmodel.Post{
		UserId:    asBotID,
		ChannelId: dmChannel.Id,
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
