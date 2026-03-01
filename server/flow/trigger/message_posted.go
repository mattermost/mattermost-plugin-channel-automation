package trigger

import (
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// MessagePostedTrigger matches when a message is posted in the configured channel.
type MessagePostedTrigger struct{}

func (t *MessagePostedTrigger) Type() string { return "message_posted" }

func (t *MessagePostedTrigger) Matches(trigger *model.Trigger, event *model.Event) bool {
	if event.Post == nil {
		return false
	}
	return trigger.ChannelID == event.Post.ChannelId
}
