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
		assert.Equal(t, *td.User, td.Post.User)
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

	t.Run("root post does not fetch thread", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.channels["ch1"] = &mmmodel.Channel{Id: "ch1", Name: "n"}
		api.users["u1"] = &mmmodel.User{Id: "u1", Username: "alice"}
		event := &model.Event{
			Type: model.TriggerTypeMessagePosted,
			Post: &mmmodel.Post{Id: "p1", ChannelId: "ch1", UserId: "u1"},
		}
		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		assert.Nil(t, td.Thread)
	})

	t.Run("reply attaches sorted SafeThread with resolved users", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.channels["ch1"] = &mmmodel.Channel{Id: "ch1", Name: "n"}
		api.users["u1"] = &mmmodel.User{Id: "u1", Username: "alice", FirstName: "Alice", LastName: "A."}
		api.users["u2"] = &mmmodel.User{Id: "u2", Username: "bob", FirstName: "Bob", LastName: "B."}
		api.postThreads["root1"] = &mmmodel.PostList{
			Order: []string{"reply2", "root1", "reply1"}, // intentionally out of order
			Posts: map[string]*mmmodel.Post{
				"root1":  {Id: "root1", ChannelId: "ch1", UserId: "u1", Message: "hello", CreateAt: 100},
				"reply1": {Id: "reply1", ChannelId: "ch1", UserId: "u2", Message: "world", CreateAt: 200, RootId: "root1"},
				"reply2": {Id: "reply2", ChannelId: "ch1", UserId: "u1", Message: "again", CreateAt: 300, RootId: "root1"},
			},
		}

		event := &model.Event{
			Type: model.TriggerTypeMessagePosted,
			Post: &mmmodel.Post{Id: "reply2", ChannelId: "ch1", UserId: "u1", RootId: "root1", Message: "again"},
		}
		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		require.NotNil(t, td.Thread)
		assert.Equal(t, "root1", td.Thread.RootID)
		assert.Equal(t, 3, td.Thread.PostCount)
		require.Len(t, td.Thread.Messages, 3)

		// Sorted by CreateAt: root1 (100), reply1 (200), reply2 (300).
		assert.Equal(t, "root1", td.Thread.Messages[0].Id)
		assert.Equal(t, "alice", td.Thread.Messages[0].User.Username)
		assert.Equal(t, "hello", td.Thread.Messages[0].Message)
		assert.Equal(t, int64(100), td.Thread.Messages[0].CreateAt)
		assert.Equal(t, "root1", td.Thread.Messages[0].ThreadId)

		assert.Equal(t, "reply1", td.Thread.Messages[1].Id)
		assert.Equal(t, "bob", td.Thread.Messages[1].User.Username)
		assert.Equal(t, "world", td.Thread.Messages[1].Message)

		assert.Equal(t, "reply2", td.Thread.Messages[2].Id)
		assert.Equal(t, "alice", td.Thread.Messages[2].User.Username)
	})

	t.Run("thread fetch error continues without thread and logs", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.channels["ch1"] = &mmmodel.Channel{Id: "ch1", Name: "n"}
		api.users["u1"] = &mmmodel.User{Id: "u1", Username: "alice"}
		// Note: postThreads has no entry for "root1", so the fake returns a 500.

		event := &model.Event{
			Type: model.TriggerTypeMessagePosted,
			Post: &mmmodel.Post{Id: "reply1", ChannelId: "ch1", UserId: "u1", RootId: "root1", Message: "x"},
		}
		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		assert.Nil(t, td.Thread)
		assert.Contains(t, api.warnCalls, "message_posted trigger: failed to fetch thread for root post, continuing without thread context")
	})

	t.Run("thread author lookup failure keeps user-id fallback", func(t *testing.T) {
		api := newFakeTriggerAPI()
		api.channels["ch1"] = &mmmodel.Channel{Id: "ch1", Name: "n"}
		api.users["u1"] = &mmmodel.User{Id: "u1", Username: "alice"}
		// u-missing is not registered → GetUser returns error.
		api.postThreads["root1"] = &mmmodel.PostList{
			Order: []string{"root1", "reply1"},
			Posts: map[string]*mmmodel.Post{
				"root1":  {Id: "root1", ChannelId: "ch1", UserId: "u1", Message: "hello", CreateAt: 100},
				"reply1": {Id: "reply1", ChannelId: "ch1", UserId: "u-missing", Message: "x", CreateAt: 200, RootId: "root1"},
			},
		}

		event := &model.Event{
			Type: model.TriggerTypeMessagePosted,
			Post: &mmmodel.Post{Id: "reply1", ChannelId: "ch1", UserId: "u1", RootId: "root1", Message: "x"},
		}
		td, err := tr.BuildTriggerData(api, event)
		require.NoError(t, err)
		require.NotNil(t, td.Thread)
		require.Len(t, td.Thread.Messages, 2)
		assert.Equal(t, "alice", td.Thread.Messages[0].User.Username)
		// Failed lookup → SafeUser carries only the user ID; AuthorDisplay() falls back to it.
		assert.Equal(t, "u-missing", td.Thread.Messages[1].User.Id)
		assert.Empty(t, td.Thread.Messages[1].User.Username)
		assert.Contains(t, api.warnCalls, "message_posted trigger: failed to resolve thread author user, falling back to user-id-only display")
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
