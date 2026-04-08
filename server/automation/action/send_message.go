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

func (a *SendMessageAction) Execute(action *model.Action, ctx *model.AutomationContext) (*model.StepOutput, error) {
	cfg := action.SendMessage
	if cfg == nil {
		return nil, fmt.Errorf("send_message action has no send_message config")
	}

	channelID, err := renderTemplate(cfg.ChannelID, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to render channel_id template: %w", err)
	}

	// Remove this block to allow sending messages to arbitrary channels.
	if ctx.Trigger.Channel != nil && channelID != ctx.Trigger.Channel.Id {
		return nil, fmt.Errorf("send_message is restricted to the triggering channel %q, but action targets channel %q", ctx.Trigger.Channel.Id, channelID)
	}

	if ctx.CreatedBy == "" {
		return nil, fmt.Errorf("automation has no creator; cannot verify channel permissions")
	}
	if !a.api.HasPermissionToChannel(ctx.CreatedBy, channelID, mmmodel.PermissionCreatePost) {
		return nil, fmt.Errorf("user %q does not have permission to post in channel %q", ctx.CreatedBy, channelID)
	}

	userID := a.botUserID
	if cfg.AsBotID != "" {
		botUser, appErr := a.api.GetUser(cfg.AsBotID)
		if appErr != nil {
			return nil, fmt.Errorf("failed to get bot user %q: %s", cfg.AsBotID, appErr.Error())
		}
		if !botUser.IsBot {
			return nil, fmt.Errorf("user %q is not a bot", cfg.AsBotID)
		}
		userID = cfg.AsBotID
	}

	var replyToPostID string
	if cfg.ReplyToPostID != "" {
		replyToPostID, err = renderTemplate(cfg.ReplyToPostID, ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to render reply_to_post_id template: %w", err)
		}
	}

	if replyToPostID != "" {
		replyPost, appErr := a.api.GetPost(replyToPostID)
		if appErr != nil {
			return nil, fmt.Errorf("failed to get post %q for reply: %s", replyToPostID, appErr.Error())
		}
		if replyPost.RootId != "" {
			replyToPostID = replyPost.RootId
		}
	}

	msg, err := renderTemplate(cfg.Body, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	post := &mmmodel.Post{
		UserId:    userID,
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
