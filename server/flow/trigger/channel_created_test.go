package trigger_test

import (
	"net/http"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestChannelCreatedTrigger_Type(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}
	assert.Equal(t, model.TriggerTypeChannelCreated, tr.Type())
}

func TestChannelCreatedTrigger_Matches_WithChannel(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}
	trig := &model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}
	event := &model.Event{
		Type:    model.TriggerTypeChannelCreated,
		Channel: &mmmodel.Channel{Id: "ch1", TeamId: "team1"},
	}

	assert.True(t, tr.Matches(trig, event))
}

func TestChannelCreatedTrigger_Matches_WrongTeam(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}
	trig := &model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}
	event := &model.Event{
		Type:    "channel_created",
		Channel: &mmmodel.Channel{Id: "ch1", TeamId: "team2"},
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestChannelCreatedTrigger_Matches_NilChannel(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}
	trig := &model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}
	event := &model.Event{
		Type:    model.TriggerTypeChannelCreated,
		Channel: nil,
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestChannelCreatedTrigger_Matches_NilConfig(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}
	trig := &model.Trigger{}
	event := &model.Event{
		Type:    model.TriggerTypeChannelCreated,
		Channel: &mmmodel.Channel{Id: "ch1", TeamId: "team1"},
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestChannelCreatedTrigger_Validate(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}

	t.Run("valid", func(t *testing.T) {
		require.NoError(t, tr.Validate(&model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{TeamID: "team1"}}, nil))
	})

	t.Run("missing team_id", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "team_id")
	})

	t.Run("missing config", func(t *testing.T) {
		require.Error(t, tr.Validate(&model.Trigger{}, nil))
	})
}

func TestChannelCreatedTrigger_BuildTriggerData(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}

	t.Run("success with creator", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.users["creator1"] = &mmmodel.User{Id: "creator1", Username: "alice"}

		event := &model.Event{
			Type: model.TriggerTypeChannelCreated,
			Channel: &mmmodel.Channel{
				Id:        "ch1",
				TeamId:    "team1",
				Name:      "new-channel",
				CreatorId: "creator1",
			},
		}

		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		require.NotNil(t, td.Channel)
		require.NotNil(t, td.User)
		assert.Equal(t, "new-channel", td.Channel.Name)
		assert.Equal(t, "alice", td.User.Username)
	})

	t.Run("no creator id leaves user nil", func(t *testing.T) {
		api := newFakeTriggerAPI()
		event := &model.Event{
			Type:    model.TriggerTypeChannelCreated,
			Channel: &mmmodel.Channel{Id: "ch1", TeamId: "team1", Name: "n"},
		}

		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		require.NotNil(t, td.Channel)
		assert.Nil(t, td.User)
	})

	t.Run("creator lookup failure is non-fatal", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.userErrors["creator1"] = mmmodel.NewAppError("fake", "boom", nil, "boom", http.StatusInternalServerError)
		event := &model.Event{
			Type:    model.TriggerTypeChannelCreated,
			Channel: &mmmodel.Channel{Id: "ch1", TeamId: "team1", Name: "n", CreatorId: "creator1"},
		}

		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		require.NotNil(t, td.Channel)
		assert.Nil(t, td.User)
		assert.NotEmpty(t, api.warnCalls, "expected warn call on creator lookup failure")
	})

	t.Run("nil channel errors", func(t *testing.T) {
		_, err := tr.BuildTriggerData(newFakeTriggerAPI(), &model.Event{Type: model.TriggerTypeChannelCreated})
		require.Error(t, err)
	})
}

func TestChannelCreatedTrigger_CandidateFlowIDs(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}

	t.Run("nil channel returns empty", func(t *testing.T) {
		ids, err := tr.CandidateFlowIDs(&stubStore{}, &model.Event{Type: model.TriggerTypeChannelCreated})
		require.NoError(t, err)
		assert.Nil(t, ids)
	})

	t.Run("returns global channel_created flow ids", func(t *testing.T) {
		st := &stubStore{channelCreatedFlowIDs: []string{"f1", "f2"}}
		event := &model.Event{Type: model.TriggerTypeChannelCreated, Channel: &mmmodel.Channel{Id: "ch1", TeamId: "team1"}}
		ids, err := tr.CandidateFlowIDs(st, event)
		require.NoError(t, err)
		assert.Equal(t, []string{"f1", "f2"}, ids)
	})
}
