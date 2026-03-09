package trigger_test

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestMembershipChangedTrigger_Type(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}
	assert.Equal(t, "membership_changed", tr.Type())
}

func TestMembershipChangedTrigger_Matches_CorrectChannel(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}
	trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}
	event := &model.Event{
		Type:             "membership_changed",
		Channel:          &mmmodel.Channel{Id: "ch1"},
		MembershipAction: "joined",
	}

	assert.True(t, tr.Matches(trig, event))
}

func TestMembershipChangedTrigger_Matches_WrongChannel(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}
	trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}
	event := &model.Event{
		Type:             "membership_changed",
		Channel:          &mmmodel.Channel{Id: "ch2"},
		MembershipAction: "joined",
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestMembershipChangedTrigger_Matches_NilChannel(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}
	trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}
	event := &model.Event{
		Type:    "membership_changed",
		Channel: nil,
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestMembershipChangedTrigger_Matches_NilConfig(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}
	trig := &model.Trigger{}
	event := &model.Event{
		Type:    "membership_changed",
		Channel: &mmmodel.Channel{Id: "ch1"},
	}

	assert.False(t, tr.Matches(trig, event))
}
