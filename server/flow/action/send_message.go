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
	cfg := action.SendMessage
	if cfg == nil {
		return nil, fmt.Errorf("send_message action has no send_message config")
	}

	channelID, err := renderTemplate(cfg.ChannelID, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to render channel_id template: %w", err)
	}

	if ctx.CreatedBy == "" {
		return nil, fmt.Errorf("flow has no creator; cannot verify channel permissions")
	}
	if !a.api.HasPermissionToChannel(ctx.CreatedBy, channelID, mmmodel.PermissionManageChannelRoles) {
		return nil, fmt.Errorf("user %q does not have permission to manage channel %q", ctx.CreatedBy, channelID)
	}

	var replyToPostID string
	if cfg.ReplyToPostID != "" {
		replyToPostID, err = renderTemplate(cfg.ReplyToPostID, ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to render reply_to_post_id template: %w", err)
		}
	}

	msg, err := renderTemplate(cfg.Body, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	post := &mmmodel.Post{
		UserId:    a.botUserID,
		ChannelId: channelID,
		RootId:    replyToPostID,
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
