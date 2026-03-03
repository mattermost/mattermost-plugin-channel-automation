package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTrigger_MessagePosted(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Type: "message_posted", ChannelID: "ch1"})
		require.NoError(t, err)
	})

	t.Run("missing channel_id", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Type: "message_posted"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "channel_id")
	})
}

func TestValidateTrigger_Schedule(t *testing.T) {
	t.Run("valid minimal", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Type: "schedule", Interval: "5m"})
		require.NoError(t, err)
	})

	t.Run("valid with start_at", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Type: "schedule", Interval: "1h", StartAt: 1700000000000})
		require.NoError(t, err)
	})

	t.Run("missing interval", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Type: "schedule"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "interval")
	})

	t.Run("unparseable interval", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Type: "schedule", Interval: "not-a-duration"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid interval")
	})

	t.Run("interval too small", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Type: "schedule", Interval: "1m"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least")
	})

	t.Run("negative start_at", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Type: "schedule", Interval: "10m", StartAt: -1})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start_at")
	})
}

func TestValidateTrigger_UnknownType(t *testing.T) {
	err := ValidateTrigger(&Trigger{Type: "unknown"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown trigger type")
}
