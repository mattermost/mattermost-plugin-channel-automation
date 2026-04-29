package trigger

import (
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
