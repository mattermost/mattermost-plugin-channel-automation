package trigger_test

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/automation/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestMessagePostedTrigger_Type(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}
	assert.Equal(t, model.TriggerTypeMessagePosted, tr.Type())
}

func TestMessagePostedTrigger_Matches_CorrectChannel(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}
	trig := &model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}
	event := &model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{ChannelId: "ch1"},
	}

	assert.True(t, tr.Matches(trig, event))
}

func TestMessagePostedTrigger_Matches_WrongChannel(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}
	trig := &model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}
	event := &model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{ChannelId: "ch2"},
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestMessagePostedTrigger_Matches_NilPost(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}
	trig := &model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}
	event := &model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: nil,
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestMessagePostedTrigger_Matches_NilConfig(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}
	trig := &model.Trigger{}
	event := &model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{ChannelId: "ch1"},
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestMessagePostedTrigger_Matches_ThreadReplyExcludedByDefault(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}
	trig := &model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}
	event := &model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{ChannelId: "ch1", RootId: "root1"},
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestMessagePostedTrigger_Matches_ThreadReplyIncludedWhenEnabled(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}
	trig := &model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1", IncludeThreadReplies: true}}
	event := &model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{ChannelId: "ch1", RootId: "root1"},
	}

	assert.True(t, tr.Matches(trig, event))
}

func TestMessagePostedTrigger_Matches_TopLevelPostMatchesWhenIncludeEnabled(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}
	trig := &model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1", IncludeThreadReplies: true}}
	event := &model.Event{
		Type: model.TriggerTypeMessagePosted,
		Post: &mmmodel.Post{ChannelId: "ch1"},
	}

	assert.True(t, tr.Matches(trig, event))
}

func TestMessagePostedTrigger_Validate(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}

	t.Run("valid", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}, nil)
		require.NoError(t, err)
	})

	t.Run("missing channel_id", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{MessagePosted: &model.MessagePostedConfig{}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "channel_id")
	})

	t.Run("missing config", func(t *testing.T) {
		err := tr.Validate(&model.Trigger{}, nil)
		require.Error(t, err)
	})
}

func TestMessagePostedTrigger_BuildTriggerData(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}

	t.Run("success", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.channels["ch1"] = &mmmodel.Channel{Id: "ch1", Name: "town-square", DisplayName: "Town Square"}
		api.users["user1"] = &mmmodel.User{Id: "user1", Username: "alice"}

		event := &model.Event{
			Type: model.TriggerTypeMessagePosted,
			Post: &mmmodel.Post{Id: "post1", ChannelId: "ch1", UserId: "user1", Message: "hello"},
		}

		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		require.NotNil(t, td.Post)
		require.NotNil(t, td.Channel)
		require.NotNil(t, td.User)
		assert.Equal(t, "post1", td.Post.Id)
		assert.Equal(t, "town-square", td.Channel.Name)
		assert.Equal(t, "alice", td.User.Username)
	})

	t.Run("nil post errors", func(t *testing.T) {
		_, err := tr.BuildTriggerData(newFakeTriggerAPI(), &model.Event{Type: model.TriggerTypeMessagePosted})
		require.Error(t, err)
	})

	t.Run("channel fetch error propagates", func(t *testing.T) {
		api := newFakeTriggerAPI()
		event := &model.Event{
			Type: model.TriggerTypeMessagePosted,
			Post: &mmmodel.Post{Id: "post1", ChannelId: "missing", UserId: "user1"},
		}
		_, err := tr.BuildTriggerData(api, event)
		require.Error(t, err)
	})

	t.Run("user fetch error propagates", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.channels["ch1"] = &mmmodel.Channel{Id: "ch1", Name: "n"}
		event := &model.Event{
			Type: model.TriggerTypeMessagePosted,
			Post: &mmmodel.Post{Id: "post1", ChannelId: "ch1", UserId: "missing"},
		}
		_, err := tr.BuildTriggerData(api, event)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get user")
	})
}

func TestMessagePostedTrigger_CandidateAutomationIDs(t *testing.T) {
	tr := &trigger.MessagePostedTrigger{}

	t.Run("nil post returns empty", func(t *testing.T) {
		ids, err := tr.CandidateAutomationIDs(&stubStore{}, &model.Event{Type: model.TriggerTypeMessagePosted})
		require.NoError(t, err)
		assert.Nil(t, ids)
	})

	t.Run("delegates to store.GetAutomationIDsForChannel", func(t *testing.T) {
		st := &stubStore{automationIDsByChannel: map[string][]string{"ch1": {"f1", "f2"}}}
		event := &model.Event{
			Type: model.TriggerTypeMessagePosted,
			Post: &mmmodel.Post{ChannelId: "ch1"},
		}
		ids, err := tr.CandidateAutomationIDs(st, event)
		require.NoError(t, err)
		assert.Equal(t, []string{"f1", "f2"}, ids)
	})
}
