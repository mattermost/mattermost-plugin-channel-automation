package trigger

import (
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// MembershipChangedTrigger matches when a user joins or leaves the configured channel.
type MembershipChangedTrigger struct{}

func (t *MembershipChangedTrigger) Type() string { return model.TriggerTypeMembershipChanged }

func (t *MembershipChangedTrigger) Matches(trigger *model.Trigger, event *model.Event) bool {
	if trigger.MembershipChanged == nil {
		return false
	}
	if event.Channel == nil {
		return false
	}
	if trigger.MembershipChanged.ChannelID != event.Channel.Id {
		return false
	}
	if a := trigger.MembershipChanged.Action; a != "" && a != event.MembershipAction {
		return false
	}
	return true
}
