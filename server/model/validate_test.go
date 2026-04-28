package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTriggerExclusivity(t *testing.T) {
	t.Run("single trigger type ok", func(t *testing.T) {
		require.NoError(t, ValidateTriggerExclusivity(&Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}}))
	})

	t.Run("two trigger types rejected", func(t *testing.T) {
		err := ValidateTriggerExclusivity(&Trigger{
			MessagePosted: &MessagePostedConfig{ChannelID: "ch1"},
			Schedule:      &ScheduleConfig{ChannelID: "ch1", Interval: "2h"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one trigger type must be set")
	})

	t.Run("three trigger types rejected", func(t *testing.T) {
		err := ValidateTriggerExclusivity(&Trigger{
			MessagePosted:     &MessagePostedConfig{ChannelID: "ch1"},
			Schedule:          &ScheduleConfig{ChannelID: "ch1", Interval: "2h"},
			MembershipChanged: &MembershipChangedConfig{ChannelID: "ch1"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one trigger type must be set, got 3")
	})

	t.Run("no trigger type rejected", func(t *testing.T) {
		err := ValidateTriggerExclusivity(&Trigger{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one trigger type must be set")
	})
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

func TestValidateSendMessageChannel(t *testing.T) {
	t.Run("message_posted with matching literal channel", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}}},
		}
		require.NoError(t, ValidateSendMessageChannel(f))
	})

	t.Run("message_posted with trigger channel template", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "{{.Trigger.Channel.Id}}", Body: "hi"}}},
		}
		require.NoError(t, ValidateSendMessageChannel(f))
	})

	t.Run("message_posted with trigger channel template with spaces", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "{{ .Trigger.Channel.Id }}", Body: "hi"}}},
		}
		require.NoError(t, ValidateSendMessageChannel(f))
	})

	t.Run("message_posted with different literal channel rejected", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "ch-other", Body: "hi"}}},
		}
		err := ValidateSendMessageChannel(f)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must reference the triggering channel")
	})

	t.Run("membership_changed with matching literal channel", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{MembershipChanged: &MembershipChangedConfig{ChannelID: "ch1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}}},
		}
		require.NoError(t, ValidateSendMessageChannel(f))
	})

	t.Run("channel_created with template", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{ChannelCreated: &ChannelCreatedConfig{TeamID: "team1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "{{.Trigger.Channel.Id}}", Body: "hi"}}},
		}
		require.NoError(t, ValidateSendMessageChannel(f))
	})

	t.Run("channel_created with literal channel rejected", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{ChannelCreated: &ChannelCreatedConfig{TeamID: "team1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "some-ch", Body: "hi"}}},
		}
		err := ValidateSendMessageChannel(f)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must use a template expression")
	})

	t.Run("schedule trigger enforces channel restriction", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "any-ch", Body: "hi"}}},
		}
		require.Error(t, ValidateSendMessageChannel(f))
	})

	t.Run("schedule trigger allows matching channel", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{Schedule: &ScheduleConfig{ChannelID: "ch1", Interval: "1h"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "ch1", Body: "hi"}}},
		}
		require.NoError(t, ValidateSendMessageChannel(f))
	})

	t.Run("non-send_message actions are ignored", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
			Actions: []Action{{ID: "a", AIPrompt: &AIPromptActionConfig{Prompt: "test", ProviderType: "agent", ProviderID: "bot1"}}},
		}
		require.NoError(t, ValidateSendMessageChannel(f))
	})

	t.Run("user_joined_team accepts Team.DefaultChannelId template", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{UserJoinedTeam: &UserJoinedTeamConfig{TeamID: "team1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "{{.Trigger.Team.DefaultChannelId}}", Body: "hi"}}},
		}
		require.NoError(t, ValidateSendMessageChannel(f))
	})

	t.Run("message_posted accepts Post.ChannelId template", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "{{.Trigger.Post.ChannelId}}", Body: "hi"}}},
		}
		require.NoError(t, ValidateSendMessageChannel(f))
	})

	t.Run("user_joined_team rejects Trigger.User.Id template", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{UserJoinedTeam: &UserJoinedTeamConfig{TeamID: "team1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "{{.Trigger.User.Id}}", Body: "hi"}}},
		}
		err := ValidateSendMessageChannel(f)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must use a template expression")
	})

	t.Run("channel_created rejects Steps template (chaining not supported)", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{ChannelCreated: &ChannelCreatedConfig{TeamID: "team1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "{{.Steps.create_ch.ChannelID}}", Body: "hi"}}},
		}
		err := ValidateSendMessageChannel(f)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must use a template expression")
	})

	t.Run("message_posted rejects template with trailing literal", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "{{.Trigger.Channel.Id}}extra", Body: "hi"}}},
		}
		err := ValidateSendMessageChannel(f)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must reference the triggering channel")
	})

	t.Run("message_posted rejects template with leading literal", func(t *testing.T) {
		f := &Flow{
			Trigger: Trigger{MessagePosted: &MessagePostedConfig{ChannelID: "ch1"}},
			Actions: []Action{{ID: "a", SendMessage: &SendMessageActionConfig{ChannelID: "prefix{{.Trigger.Channel.Id}}", Body: "hi"}}},
		}
		err := ValidateSendMessageChannel(f)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must reference the triggering channel")
	})
}
