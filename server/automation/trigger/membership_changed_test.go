package trigger_test

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestMembershipChangedTrigger_Type(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}
	assert.Equal(t, model.TriggerTypeMembershipChanged, tr.Type())
}

func TestMembershipChangedTrigger_Matches_CorrectChannel(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}
	trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}
	event := &model.Event{
		Type:             model.TriggerTypeMembershipChanged,
		Channel:          &mmmodel.Channel{Id: "ch1"},
		MembershipAction: "joined",
	}

	assert.True(t, tr.Matches(trig, event))
}

func TestMembershipChangedTrigger_Matches_WrongChannel(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}
	trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}
	event := &model.Event{
		Type:             model.TriggerTypeMembershipChanged,
		Channel:          &mmmodel.Channel{Id: "ch2"},
		MembershipAction: "joined",
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestMembershipChangedTrigger_Matches_NilChannel(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}
	trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}
	event := &model.Event{
		Type:    model.TriggerTypeMembershipChanged,
		Channel: nil,
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestMembershipChangedTrigger_Matches_NilConfig(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}
	trig := &model.Trigger{}
	event := &model.Event{
		Type:    model.TriggerTypeMembershipChanged,
		Channel: &mmmodel.Channel{Id: "ch1"},
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestMembershipChangedTrigger_Matches_ActionFilter(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}

	t.Run("empty action matches both joined and left", func(t *testing.T) {
		trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1", Action: ""}}
		joinEvent := &model.Event{Type: model.TriggerTypeMembershipChanged, Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "joined"}
		leaveEvent := &model.Event{Type: model.TriggerTypeMembershipChanged, Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "left"}

		assert.True(t, tr.Matches(trig, joinEvent))
		assert.True(t, tr.Matches(trig, leaveEvent))
	})

	t.Run("joined action matches only joined", func(t *testing.T) {
		trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1", Action: "joined"}}
		joinEvent := &model.Event{Type: model.TriggerTypeMembershipChanged, Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "joined"}
		leaveEvent := &model.Event{Type: model.TriggerTypeMembershipChanged, Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "left"}

		assert.True(t, tr.Matches(trig, joinEvent))
		assert.False(t, tr.Matches(trig, leaveEvent))
	})

	t.Run("left action matches only left", func(t *testing.T) {
		trig := &model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1", Action: "left"}}
		joinEvent := &model.Event{Type: model.TriggerTypeMembershipChanged, Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "joined"}
		leaveEvent := &model.Event{Type: model.TriggerTypeMembershipChanged, Channel: &mmmodel.Channel{Id: "ch1"}, MembershipAction: "left"}

		assert.False(t, tr.Matches(trig, joinEvent))
		assert.True(t, tr.Matches(trig, leaveEvent))
	})
}

func TestMembershipChangedTrigger_Validate(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}

	t.Run("valid without action", func(t *testing.T) {
		require.NoError(t, tr.Validate(&model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1"}}, nil))
	})

	for _, action := range []string{"joined", "left", ""} {
		t.Run("valid action "+action, func(t *testing.T) {
			require.NoError(t, tr.Validate(&model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1", Action: action}}, nil))
		})
	}

	t.Run("invalid action", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{MembershipChanged: &model.MembershipChangedConfig{ChannelID: "ch1", Action: "kicked"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "action")
	})

	t.Run("missing channel_id", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{MembershipChanged: &model.MembershipChangedConfig{}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "channel_id")
	})

	t.Run("missing config", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{}, nil)
		require.Error(t, err)
	})
}

func TestMembershipChangedTrigger_BuildTriggerData(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}

	t.Run("success", func(t *testing.T) {
		event := &model.Event{
			Type:             model.TriggerTypeMembershipChanged,
			Channel:          &mmmodel.Channel{Id: "ch1", Name: "town-square"},
			User:             &mmmodel.User{Id: "user1", Username: "alice"},
			MembershipAction: "joined",
		}

		td, err := tr.BuildTriggerData(newFakeTriggerAPI(), event)
		require.NoError(t, err)
		require.NotNil(t, td.Channel)
		require.NotNil(t, td.User)
		require.NotNil(t, td.Membership)
		assert.Equal(t, "town-square", td.Channel.Name)
		assert.Equal(t, "alice", td.User.Username)
		assert.Equal(t, "joined", td.Membership.Action)
	})

	t.Run("missing channel errors", func(t *testing.T) {
		_, err := tr.BuildTriggerData(newFakeTriggerAPI(), &model.Event{Type: model.TriggerTypeMembershipChanged, User: &mmmodel.User{Id: "u"}})
		require.Error(t, err)
	})

	t.Run("missing user errors", func(t *testing.T) {
		_, err := tr.BuildTriggerData(newFakeTriggerAPI(), &model.Event{Type: model.TriggerTypeMembershipChanged, Channel: &mmmodel.Channel{Id: "c"}})
		require.Error(t, err)
	})
}

func TestMembershipChangedTrigger_CandidateAutomationIDs(t *testing.T) {
	tr := &trigger.MembershipChangedTrigger{}

	t.Run("nil channel returns empty", func(t *testing.T) {
		ids, err := tr.CandidateAutomationIDs(&stubStore{}, &model.Event{Type: model.TriggerTypeMembershipChanged})
		require.NoError(t, err)
		assert.Nil(t, ids)
	})

	t.Run("delegates to store index", func(t *testing.T) {
		st := &stubStore{automationIDsByMembershipChannel: map[string][]string{"ch1": {"f1"}}}
		event := &model.Event{Type: model.TriggerTypeMembershipChanged, Channel: &mmmodel.Channel{Id: "ch1"}}
		ids, err := tr.CandidateAutomationIDs(st, event)
		require.NoError(t, err)
		assert.Equal(t, []string{"f1"}, ids)
	})
}
