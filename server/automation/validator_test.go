package automation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

func TestValidateTrigger(t *testing.T) {
	registry := newTestRegistry()

	t.Run("valid message_posted", func(t *testing.T) {
		err := ValidateTrigger(registry, &model.Trigger{MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"}}, nil)
		require.NoError(t, err)
	})

	t.Run("mutual exclusion before per-type checks", func(t *testing.T) {
		err := ValidateTrigger(registry, &model.Trigger{
			MessagePosted: &model.MessagePostedConfig{ChannelID: "ch1"},
			Schedule:      &model.ScheduleConfig{ChannelID: "ch1", Interval: "2h"},
		}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one trigger type must be set")
	})

	t.Run("per-type check is dispatched", func(t *testing.T) {
		err := ValidateTrigger(registry, &model.Trigger{MessagePosted: &model.MessagePostedConfig{}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "channel_id")
	})

	t.Run("empty trigger rejected", func(t *testing.T) {
		err := ValidateTrigger(registry, &model.Trigger{}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one trigger type must be set")
	})

	t.Run("unknown type rejected when handler missing", func(t *testing.T) {
		// A registry without the schedule trigger should reject schedule configs.
		emptyRegistry := NewRegistry()
		err := ValidateTrigger(emptyRegistry, &model.Trigger{Schedule: &model.ScheduleConfig{ChannelID: "ch1", Interval: "1h"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown trigger type")
	})
}
