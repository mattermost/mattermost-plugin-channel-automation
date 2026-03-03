package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollectChannelIDs_LiteralChannels(t *testing.T) {
	f := &Flow{
		Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch2"}},
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch3"}},
		},
	}

	ids := CollectChannelIDs(f)
	assert.Equal(t, []string{"ch1", "ch2", "ch3"}, ids)
}

func TestCollectChannelIDs_TemplatedChannelSkipped(t *testing.T) {
	f := &Flow{
		Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "{{.Trigger.Channel.Id}}"}},
		},
	}

	ids := CollectChannelIDs(f)
	assert.Equal(t, []string{"ch1"}, ids)
}

func TestCollectChannelIDs_DuplicatesRemoved(t *testing.T) {
	f := &Flow{
		Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch1"}},
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch2"}},
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch2"}},
		},
	}

	ids := CollectChannelIDs(f)
	assert.Equal(t, []string{"ch1", "ch2"}, ids)
}

func TestCollectChannelIDs_ScheduleTrigger(t *testing.T) {
	f := &Flow{
		Trigger: Trigger{Schedule: &ScheduleConfig{Interval: "1h"}},
		Actions: []Action{
			{SendMessage: &SendMessageActionConfig{ChannelID: "ch1"}},
		},
	}

	ids := CollectChannelIDs(f)
	assert.Equal(t, []string{"ch1"}, ids)
}

func TestCollectChannelIDs_NoChannels(t *testing.T) {
	f := &Flow{
		Trigger: Trigger{Schedule: &ScheduleConfig{Interval: "1h"}},
		Actions: []Action{
			{AIPrompt: &AIPromptActionConfig{Prompt: "test"}},
		},
	}

	ids := CollectChannelIDs(f)
	assert.Empty(t, ids)
}
