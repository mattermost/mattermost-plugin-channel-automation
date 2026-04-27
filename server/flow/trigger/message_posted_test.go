package trigger_test

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/trigger"
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
