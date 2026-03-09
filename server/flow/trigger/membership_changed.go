package trigger

import (
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// MembershipChangedTrigger matches when a user joins or leaves the configured channel.
type MembershipChangedTrigger struct{}

func (t *MembershipChangedTrigger) Type() string { return "membership_changed" }

func (t *MembershipChangedTrigger) Matches(trigger *model.Trigger, event *model.Event) bool {
	if trigger.MembershipChanged == nil {
		return false
	}
	if event.Channel == nil {
		return false
	}
	return trigger.MembershipChanged.ChannelID == event.Channel.Id
}
