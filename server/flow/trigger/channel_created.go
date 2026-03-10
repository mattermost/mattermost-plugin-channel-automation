package trigger

import (
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// ChannelCreatedTrigger matches when a new public channel is created.
type ChannelCreatedTrigger struct{}

func (t *ChannelCreatedTrigger) Type() string { return "channel_created" }

func (t *ChannelCreatedTrigger) Matches(trigger *model.Trigger, event *model.Event) bool {
	if trigger.ChannelCreated == nil {
		return false
	}
	return event.Channel != nil
}
