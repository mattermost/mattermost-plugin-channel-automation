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

func TestMembershipChangedTrigger_Matches_ActionFilter(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}

	t.Run("empty action matches both joined and left", func(t *testing.T) {
		trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1", Action: ""}}
		joinEvent := &model.Event{Type: "membership_changed", Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "joined"}
		leaveEvent := &model.Event{Type: "membership_changed", Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "left"}

		assert.True(t, tr.Matches(trig, joinEvent))
		assert.True(t, tr.Matches(trig, leaveEvent))
	})

	t.Run("joined action matches only joined", func(t *testing.T) {
		trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1", Action: "joined"}}
		joinEvent := &model.Event{Type: "membership_changed", Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "joined"}
		leaveEvent := &model.Event{Type: "membership_changed", Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "left"}

		assert.True(t, tr.Matches(trig, joinEvent))
		assert.False(t, tr.Matches(trig, leaveEvent))
	})

	t.Run("left action matches only left", func(t *testing.T) {
		trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1", Action: "left"}}
		joinEvent := &model.Event{Type: "membership_changed", Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "joined"}
		leaveEvent := &model.Event{Type: "membership_changed", Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "left"}

		assert.False(t, tr.Matches(trig, joinEvent))
		assert.True(t, tr.Matches(trig, leaveEvent))
	})
}
