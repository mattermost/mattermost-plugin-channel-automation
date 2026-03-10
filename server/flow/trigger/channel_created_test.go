package trigger_test

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/flow/trigger"
	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestChannelCreatedTrigger_Type(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}
	assert.Equal(t, "channel_created", tr.Type())
}

func TestChannelCreatedTrigger_Matches_WithChannel(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}
	trig := &model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{}}
	event := &model.Event{
		Type:    "channel_created",
		Channel: &mmmodel.Channel{Id: "ch1"},
	}

	assert.True(t, tr.Matches(trig, event))
}

func TestChannelCreatedTrigger_Matches_NilChannel(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}
	trig := &model.Trigger{ChannelCreated: &model.ChannelCreatedConfig{}}
	event := &model.Event{
		Type:    "channel_created",
		Channel: nil,
	}

	assert.False(t, tr.Matches(trig, event))
}

func TestChannelCreatedTrigger_Matches_NilConfig(t *testing.T) {
	tr := &trigger.ChannelCreatedTrigger{}
	trig := &model.Trigger{}
	event := &model.Event{
		Type:    "channel_created",
		Channel: &mmmodel.Channel{Id: "ch1"},
	}

	assert.False(t, tr.Matches(trig, event))
}
