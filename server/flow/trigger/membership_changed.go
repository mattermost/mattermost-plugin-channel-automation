package trigger

import (
	"fmt"

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

func (t *MembershipChangedTrigger) Validate(trigger *model.Trigger, _ *model.Trigger) error {
	if trigger.MembershipChanged == nil {
		return fmt.Errorf("membership_changed trigger config is missing")
	}
	if trigger.MembershipChanged.ChannelID == "" {
		return fmt.Errorf("membership_changed trigger requires channel_id")
	}
	if a := trigger.MembershipChanged.Action; a != "" && a != "joined" && a != "left" {
		return fmt.Errorf("membership_changed trigger action must be \"joined\", \"left\", or empty (both)")
	}
	return nil
}

func (t *MembershipChangedTrigger) CandidateFlowIDs(store model.Store, event *model.Event) ([]string, error) {
	if event.Channel == nil {
		return nil, nil
	}
	return store.GetFlowIDsForMembershipChannel(event.Channel.Id)
}

func (t *MembershipChangedTrigger) BuildTriggerData(_ model.TriggerAPI, event *model.Event) (model.TriggerData, error) {
	if event.Channel == nil || event.User == nil {
		return model.TriggerData{}, fmt.Errorf("membership_changed event missing channel or user")
	}
	return model.TriggerData{
		Channel:    model.NewSafeChannel(event.Channel),
		User:       model.NewSafeUser(event.User),
		Membership: &model.MembershipInfo{Action: event.MembershipAction},
	}, nil
}
