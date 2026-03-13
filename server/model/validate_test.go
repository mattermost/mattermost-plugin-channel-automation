package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTrigger_MessagePosted(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}}, nil)
		require.NoError(t, err)
	})

	t.Run("missing channel_id", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{MessagePosted: &MessagePostedConfig{}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "channel_id")
	})
}

func TestValidateTrigger_Schedule(t *testing.T) {
	t.Run("valid minimal", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "5m"}}, nil)
		require.NoError(t, err)
	})

	t.Run("valid with start_at", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "1h", StartAt: time.Now().Add(1 * time.Hour).UnixMilli()}}, nil)
		require.NoError(t, err)
	})

	t.Run("missing channel_id", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Schedule: &ScheduleConfig{Interval: "5m"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "channel_id")
	})

	t.Run("missing interval", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "interval")
	})

	t.Run("unparseable interval", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "not-a-duration"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid interval")
	})

	t.Run("interval too small", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "1m"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least")
	})

	t.Run("start_at in the past", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "10m", StartAt: time.Now().Add(-1 * time.Hour).UnixMilli()}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start_at")
	})

	t.Run("update with unchanged past start_at is valid", func(t *testing.T) {
		pastStartAt := time.Now().Add(-1 * time.Hour).UnixMilli()
		existing := &Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "10m", StartAt: pastStartAt}}
		updated := &Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "10m", StartAt: pastStartAt}}
		err := ValidateTrigger(updated, existing)
		require.NoError(t, err)
	})

	t.Run("update with round-tripped past start_at is valid", func(t *testing.T) {
		// The webapp truncates to minute precision via datetime-local input;
		// a round-tripped value may differ by up to 59s but should still be
		// treated as unchanged.
		pastStartAt := time.Now().Add(-1 * time.Hour).UnixMilli()
		truncated := time.UnixMilli(pastStartAt).Truncate(time.Minute).UnixMilli()
		existing := &Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "10m", StartAt: pastStartAt}}
		updated := &Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "10m", StartAt: truncated}}
		err := ValidateTrigger(updated, existing)
		require.NoError(t, err)
	})

	t.Run("update with new past start_at is rejected", func(t *testing.T) {
		existing := &Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "10m", StartAt: time.Now().Add(-2 * time.Hour).UnixMilli()}}
		updated := &Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "10m", StartAt: time.Now().Add(-1 * time.Hour).UnixMilli()}}
		err := ValidateTrigger(updated, existing)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start_at")
	})
}

func TestValidateTrigger_MembershipChanged(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{MembershipChanged: &MembershipChangedConfig{ChannelID: "ch1"}}, nil)
		require.NoError(t, err)
	})

	t.Run("valid with joined action", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{MembershipChanged: &MembershipChangedConfig{ChannelID: "ch1", Action: "joined"}}, nil)
		require.NoError(t, err)
	})

	t.Run("valid with left action", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{MembershipChanged: &MembershipChangedConfig{ChannelID: "ch1", Action: "left"}}, nil)
		require.NoError(t, err)
	})

	t.Run("valid with empty action", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{MembershipChanged: &MembershipChangedConfig{ChannelID: "ch1", Action: ""}}, nil)
		require.NoError(t, err)
	})

	t.Run("invalid action", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{MembershipChanged: &MembershipChangedConfig{ChannelID: "ch1", Action: "kicked"}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "action")
	})

	t.Run("missing channel_id", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{MembershipChanged: &MembershipChangedConfig{}}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "channel_id")
	})
}

func TestValidateTrigger_NoTriggerType(t *testing.T) {
	err := ValidateTrigger(&Trigger{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one trigger type must be set")
}

func TestValidateActions(t *testing.T) {
	validAction := func(id string) Action {
		return Action{ID: id, SendMessage: &SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}}
	}

	t.Run("valid single action", func(t *testing.T) {
		err := ValidateActions([]Action{validAction("send-greeting")})
		require.NoError(t, err)
	})

	t.Run("valid multiple actions", func(t *testing.T) {
		err := ValidateActions([]Action{validAction("step-1"), validAction("step-2")})
		require.NoError(t, err)
	})

	t.Run("valid id patterns", func(t *testing.T) {
		for _, id := range []string{"a", "abc", "a1", "send-message", "step-1-done", "x0-y1-z2"} {
			err := ValidateActions([]Action{validAction(id)})
			require.NoError(t, err, "expected valid: %s", id)
		}
	})

	t.Run("empty id", func(t *testing.T) {
		err := ValidateActions([]Action{{ID: "", SendMessage: &SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("invalid id patterns", func(t *testing.T) {
		for _, id := range []string{"Send", "UPPER", "has space", "trailing-", "-leading", "double--hyphen", "123", "1abc", "has_underscore", "has.dot"} {
			err := ValidateActions([]Action{{ID: id, SendMessage: &SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}}})
			require.Error(t, err, "expected invalid: %s", id)
			assert.Contains(t, err.Error(), "invalid")
		}
	})

	t.Run("duplicate ids", func(t *testing.T) {
		err := ValidateActions([]Action{validAction("send-msg"), validAction("send-msg")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate")
	})

	t.Run("missing action config", func(t *testing.T) {
		err := ValidateActions([]Action{{ID: "no-config"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "action config must be set")
	})

	t.Run("empty list is rejected", func(t *testing.T) {
		err := ValidateActions([]Action{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one action")
	})

	t.Run("nil list is rejected", func(t *testing.T) {
		err := ValidateActions(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one action")
	})

	t.Run("multiple action configs rejected", func(t *testing.T) {
		err := ValidateActions([]Action{{
			ID:          "multi",
			SendMessage: &SendMessageActionConfig{ChannelID: "ch1", Body: "hi"},
			AIPrompt:    &AIPromptActionConfig{Prompt: "test", ProviderType: "agent", ProviderID: "bot1"},
		}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one action config must be set")
	})
}

func TestValidateTrigger_MutualExclusion(t *testing.T) {
	t.Run("two trigger types rejected", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{
			MessagePosted: &MessagePostedConfig{ChannelID: "ch1"},
			Schedule:      &ScheduleConfig{ChannelID: "ch1", Interval: "10m"},
		}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one trigger type must be set")
	})

	t.Run("three trigger types rejected", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{
			MessagePosted:     &MessagePostedConfig{ChannelID: "ch1"},
			Schedule:          &ScheduleConfig{ChannelID: "ch1", Interval: "10m"},
			MembershipChanged: &MembershipChangedConfig{ChannelID: "ch1"},
		}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one trigger type must be set, got 3")
	})

	t.Run("no trigger type rejected", func(t *testing.T) {
		err := ValidateTrigger(&Trigger{}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one trigger type must be set")
	})
}
