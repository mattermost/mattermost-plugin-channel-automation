package trigger

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// MessagePostedTrigger matches when a message is posted in the configured channel.
type MessagePostedTrigger struct{}

func (t *MessagePostedTrigger) Type() string { return model.TriggerTypeMessagePosted }

func (t *MessagePostedTrigger) Matches(trigger *model.Trigger, event *model.Event) bool {
	if event.Post == nil {
		return false
	}
	if trigger.MessagePosted == nil {
		return false
	}
	if trigger.MessagePosted.ChannelID != event.Post.ChannelId {
		return false
	}
	if event.Post.RootId != "" && !trigger.MessagePosted.IncludeThreadReplies {
		return false
	}
	return true
}

func (t *MessagePostedTrigger) Validate(trigger *model.Trigger, _ *model.Trigger) error {
	if trigger.MessagePosted == nil {
		return fmt.Errorf("message_posted trigger config is missing")
	}
	if trigger.MessagePosted.ChannelID == "" {
		return fmt.Errorf("message_posted trigger requires channel_id")
	}
	return nil
}

func (t *MessagePostedTrigger) CandidateAutomationIDs(store model.Store, event *model.Event) ([]string, error) {
	if event.Post == nil {
		return nil, nil
	}
	return store.GetAutomationIDsForChannel(event.Post.ChannelId)
}

func (t *MessagePostedTrigger) BuildTriggerData(api model.TriggerAPI, event *model.Event) (model.TriggerData, error) {
	if event.Post == nil {
		return model.TriggerData{}, fmt.Errorf("message_posted event has no post")
	}

	channel, appErr := api.GetChannel(event.Post.ChannelId)
	if appErr != nil {
		return model.TriggerData{}, fmt.Errorf("get channel %s: %w", event.Post.ChannelId, appErr)
	}
	user, appErr := api.GetUser(event.Post.UserId)
	if appErr != nil {
		return model.TriggerData{}, fmt.Errorf("get user %s: %w", event.Post.UserId, appErr)
	}

	return model.TriggerData{
		Post:    model.NewSafePost(event.Post),
		Channel: model.NewSafeChannel(channel),
		User:    model.NewSafeUser(user),
	}, nil
}
